# 可靠性手动验证

本文档用于手动演示订单链路的可靠性能力。命令以 PowerShell 为例，使用项目当前真实接口和队列名。

## 0. 启动顺序

准备配置：

```powershell
Copy-Item .env.example .env
Copy-Item config/config.example.yaml config/config.yaml
```

按实际环境修改 `.env` 和 `config/config.yaml` 中的 MySQL、RabbitMQ、JWT 配置。

启动基础设施：

```powershell
docker compose up -d mysql redis rabbitmq
```

按顺序启动服务，分别打开 4 个 PowerShell 窗口：

```powershell
cd "d:\desk top\micro-system\user-service"
go run .
```

```powershell
cd "d:\desk top\micro-system\product-service"
go run .
```

```powershell
cd "d:\desk top\micro-system\order-service"
go run .
```

```powershell
cd "d:\desk top\micro-system\api-gateway"
go run .
```

设置后续命令变量。下面示例使用 `.env.example` 的默认账号，请替换成你的真实值：

```powershell
$RabbitUser = "change_me"
$RabbitPass = "change_me"
$MysqlPass = "change_me"
$BaseUrl = "http://localhost:8080"
```

注册并登录：

```powershell
curl.exe -X POST "$BaseUrl/api/v1/public/register" `
  -H "Content-Type: application/json" `
  -d "{\"username\":\"alice\",\"password\":\"123456\"}"

$login = curl.exe -s -X POST "$BaseUrl/api/v1/public/login" `
  -H "Content-Type: application/json" `
  -d "{\"username\":\"alice\",\"password\":\"123456\"}" | ConvertFrom-Json

$Token = $login.token
```

RabbitMQ 管理页面可用于观察队列：

```text
http://localhost:15672
```

涉及的队列：

```text
stock_queue
stock_result_queue
stock.deduct.retry.queue
stock.deduct.dead.queue
```

## 1. 正常下单：pending_stock -> confirmed

准备数据：确保商品 1 有库存，并同步 Redis 库存缓存。

```powershell
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "UPDATE products SET stock=10 WHERE id=1;"
docker exec micro-redis redis-cli SET product_stock:1 10
docker exec micro-redis redis-cli DEL product:1
```

执行下单：

```powershell
$order = curl.exe -s -X POST "$BaseUrl/api/v1/auth/order" `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer $Token" `
  -d "{\"product_id\":1}" | ConvertFrom-Json

$OrderId = $order.order_id
Start-Sleep -Seconds 2
curl.exe "$BaseUrl/api/v1/auth/order/$OrderId" -H "Authorization: Bearer $Token"
```

预期结果：

- 订单最初由 order-service 创建为 `pending_stock`。
- product-service 扣减库存成功。
- order-service 消费库存成功结果后，订单状态变为 `confirmed`。
- 响应中 `status` 为 `confirmed`，`status_message` 为 `order confirmed`。

## 2. 库存不足：pending_stock -> failed

准备数据：将商品 1 的 MySQL 和 Redis 库存都设为 0。

```powershell
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "UPDATE products SET stock=0 WHERE id=1;"
docker exec micro-redis redis-cli SET product_stock:1 0
docker exec micro-redis redis-cli DEL product:1
```

执行下单：

```powershell
$order = curl.exe -s -X POST "$BaseUrl/api/v1/auth/order" `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer $Token" `
  -d "{\"product_id\":1}" | ConvertFrom-Json

$FailedOrderId = $order.order_id
Start-Sleep -Seconds 2
curl.exe "$BaseUrl/api/v1/auth/order/$FailedOrderId" -H "Authorization: Bearer $Token"
```

预期结果：

- 订单先进入 `pending_stock`。
- product-service 判断库存不足，直接发送库存失败结果，不进入无限 retry。
- order-service 将订单更新为 `failed`。
- 响应中 `status` 为 `failed`，`status_message` 通常为 `insufficient stock`。

## 3. 重复投递同一个库存扣减消息，不会重复扣库存

准备数据：恢复库存，并清理本场景使用的幂等记录。

```powershell
$DupOrderId = "ORD-IDEMPOTENT-1"
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "UPDATE products SET stock=10 WHERE id=1; DELETE FROM stock_deduction_logs WHERE order_id='$DupOrderId';"
docker exec micro-redis redis-cli SET product_stock:1 10
docker exec micro-redis redis-cli DEL "processed_order:$DupOrderId"
```

向 `stock_queue` 重复发布同一条库存扣减消息两次：

```powershell
$payload = @{
  properties = @{}
  routing_key = "stock_queue"
  payload = "{\"order_id\":\"$DupOrderId\",\"user_id\":1,\"product_id\":1,\"retry_count\":0}"
  payload_encoding = "string"
} | ConvertTo-Json -Compress

curl.exe -u "${RabbitUser}:${RabbitPass}" -H "Content-Type: application/json" `
  -X POST http://localhost:15672/api/exchanges/%2F/amq.default/publish `
  -d $payload

curl.exe -u "${RabbitUser}:${RabbitPass}" -H "Content-Type: application/json" `
  -X POST http://localhost:15672/api/exchanges/%2F/amq.default/publish `
  -d $payload

Start-Sleep -Seconds 3
```

查看库存和扣减日志：

```powershell
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "SELECT id, stock FROM products WHERE id=1; SELECT order_id, product_id, quantity, status, reason FROM stock_deduction_logs WHERE order_id='$DupOrderId';"
```

预期结果：

- 商品库存只从 10 变为 9，不会扣两次。
- `stock_deduction_logs` 中只有一条 `order_id = ORD-IDEMPOTENT-1` 的记录。
- 第二次重复消息会被 MySQL `unique(order_id)` 或 Redis 幂等 key 拦住并 ack。

## 4. confirmed 后再收到重复失败结果，不会覆盖状态

准备数据：先创建一个能成功 confirmed 的订单。

```powershell
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "UPDATE products SET stock=10 WHERE id=1;"
docker exec micro-redis redis-cli SET product_stock:1 10

$order = curl.exe -s -X POST "$BaseUrl/api/v1/auth/order" `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer $Token" `
  -d "{\"product_id\":1}" | ConvertFrom-Json

$ConfirmedOrderId = $order.order_id
Start-Sleep -Seconds 2
curl.exe "$BaseUrl/api/v1/auth/order/$ConfirmedOrderId" -H "Authorization: Bearer $Token"
```

向 `stock_result_queue` 手动发布一条重复失败结果：

```powershell
$resultPayload = @{
  properties = @{}
  routing_key = "stock_result_queue"
  payload = "{\"order_id\":\"$ConfirmedOrderId\",\"user_id\":1,\"product_id\":1,\"success\":false,\"reason\":\"duplicate late failure\"}"
  payload_encoding = "string"
} | ConvertTo-Json -Compress

curl.exe -u "${RabbitUser}:${RabbitPass}" -H "Content-Type: application/json" `
  -X POST http://localhost:15672/api/exchanges/%2F/amq.default/publish `
  -d $resultPayload

Start-Sleep -Seconds 2
curl.exe "$BaseUrl/api/v1/auth/order/$ConfirmedOrderId" -H "Authorization: Bearer $Token"
```

预期结果：

- 订单仍为 `confirmed`。
- 重复失败结果不会覆盖最终状态。
- order-service 日志应出现 `order status update skipped`，原因是条件更新只允许 `pending_stock -> confirmed/failed`。

## 5. product-service 临时异常，消息进入 retry

准备数据：临时制造 Redis 异常。先停止 Redis 容器。

```powershell
docker stop micro-redis
```

发布一条库存扣减消息到 `stock_queue`：

```powershell
$RetryOrderId = "ORD-RETRY-1"
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "DELETE FROM stock_deduction_logs WHERE order_id='$RetryOrderId';"

$retryPayload = @{
  properties = @{}
  routing_key = "stock_queue"
  payload = "{\"order_id\":\"$RetryOrderId\",\"user_id\":1,\"product_id\":1,\"retry_count\":0}"
  payload_encoding = "string"
} | ConvertTo-Json -Compress

curl.exe -u "${RabbitUser}:${RabbitPass}" -H "Content-Type: application/json" `
  -X POST http://localhost:15672/api/exchanges/%2F/amq.default/publish `
  -d $retryPayload

Start-Sleep -Seconds 3
```

查看 retry queue：

```powershell
curl.exe -u "${RabbitUser}:${RabbitPass}" http://localhost:15672/api/queues/%2F/stock.deduct.retry.queue
```

product-service 会同时消费 `stock.deduct.retry.queue`，所以 retry queue 的消息可能很快被继续消费。更稳定的观察方式是看 product-service 日志里的 `next_action=retry`，或查看 Prometheus 指标：

```powershell
curl.exe http://localhost:9102/metrics | Select-String "mq_retry_total"
```

预期结果：

- product-service 处理消息时 Redis 访问失败。
- 当前消息被 ack。
- 新消息发布到 `stock.deduct.retry.queue`，消息体里 `retry_count` 增加；如果 retry consumer 很快继续处理，队列深度可能已经回到 0。
- product-service 日志包含 `next_action=retry`。

恢复 Redis：

```powershell
docker start micro-redis
```

## 6. retry 超过 3 次，消息进入 dead queue

准备数据：继续让 Redis 保持不可用。

```powershell
docker stop micro-redis
```

清理并发布一条新的测试消息：

```powershell
$DeadOrderId = "ORD-DEAD-1"
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "DELETE FROM stock_deduction_logs WHERE order_id='$DeadOrderId';"

$deadPayload = @{
  properties = @{}
  routing_key = "stock_queue"
  payload = "{\"order_id\":\"$DeadOrderId\",\"user_id\":1,\"product_id\":1,\"retry_count\":0}"
  payload_encoding = "string"
} | ConvertTo-Json -Compress

curl.exe -u "${RabbitUser}:${RabbitPass}" -H "Content-Type: application/json" `
  -X POST http://localhost:15672/api/exchanges/%2F/amq.default/publish `
  -d $deadPayload

Start-Sleep -Seconds 8
```

查看 dead queue：

```powershell
curl.exe -u "${RabbitUser}:${RabbitPass}" http://localhost:15672/api/queues/%2F/stock.deduct.dead.queue
```

查看扣减日志：

```powershell
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "SELECT order_id, status, reason FROM stock_deduction_logs WHERE order_id='$DeadOrderId';"
```

预期结果：

- 消息最多重试 3 次。
- 超过最大重试次数后进入 `stock.deduct.dead.queue`。
- product-service 日志包含 `next_action=dead`。
- 如果 MySQL 可用，`stock_deduction_logs.status` 为 `failed`，`reason` 包含 `stock deduction retry exhausted`。

恢复 Redis：

```powershell
docker start micro-redis
docker exec micro-redis redis-cli SET product_stock:1 10
```

## 7. 查询订单详情接口，看到订单状态和库存处理结果

准备数据：创建一个订单并等待库存处理完成。

```powershell
docker exec micro-mysql mysql -uroot -p$MysqlPass micro_mall -e "UPDATE products SET stock=10 WHERE id=1;"
docker exec micro-redis redis-cli SET product_stock:1 10

$order = curl.exe -s -X POST "$BaseUrl/api/v1/auth/order" `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer $Token" `
  -d "{\"product_id\":1}" | ConvertFrom-Json

$DetailOrderId = $order.order_id
Start-Sleep -Seconds 2
```

查询详情：

```powershell
curl.exe "$BaseUrl/api/v1/auth/orders/$DetailOrderId/detail" `
  -H "Authorization: Bearer $Token"
```

预期结果：

- 返回 `order_id`、`user_id`、`status`。
- 返回 `product.id/name/price`。
- 返回 `stock_result.status` 和 `stock_result.reason`。
- 只能查询当前 token 所属用户的订单；查询其他用户订单应返回 `403`。

## 8. 查看 Prometheus 指标计数变化

准备数据：先执行前面的正常下单、库存不足、retry/dead 场景。

查看 order-service 指标：

```powershell
curl.exe http://localhost:9103/metrics | Select-String "orders_created_total|orders_confirmed_total|orders_failed_total|order_status_update_total|mq_consume_total"
```

查看 product-service 指标：

```powershell
curl.exe http://localhost:9102/metrics | Select-String "stock_deduct_attempt_total|stock_deduct_success_total|stock_deduct_failed_total|mq_consume_total|mq_retry_total|mq_dead_total"
```

预期结果：

- 正常下单后，`orders_created_total`、`orders_confirmed_total`、`stock_deduct_success_total` 增加。
- 库存不足后，`orders_failed_total`、`stock_deduct_failed_total{reason="insufficient stock"}` 增加。
- 消息消费后，`mq_consume_total{queue="stock_queue"}` 或 `mq_consume_total{queue="stock_result_queue"}` 增加。
- 临时异常进入 retry 后，`mq_retry_total{queue="stock.deduct.retry.queue"}` 增加。
- 超过 3 次进入 dead queue 后，`mq_dead_total{queue="stock.deduct.dead.queue"}` 增加。
