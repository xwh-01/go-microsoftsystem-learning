# Prometheus 指标说明

order-service 和 product-service 暴露 Prometheus `/metrics`：

```text
order-service   http://localhost:9103/metrics
product-service http://localhost:9102/metrics
```

端口可通过配置覆盖：

```yaml
order_service:
  metrics_port: ":9103"

product_service:
  metrics_port: ":9102"
```

## order-service

`orders_created_total`

订单记录成功写入 MySQL 后递增。此时订单状态为 `pending_stock`。

`orders_confirmed_total`

库存结果消息把订单从 `pending_stock` 成功更新为 `confirmed` 后递增。

`orders_failed_total`

订单进入 `failed` 后递增。包括库存结果失败，以及创建订单后发布库存扣减消息失败时的本地失败更新。

`order_status_update_total{from_status,to_status,result}`

订单状态更新尝试计数。

- `from_status`：当前期望来源状态，目前是 `pending_stock`。
- `to_status`：目标状态，如 `confirmed`、`failed`。
- `result=applied`：数据库条件更新命中。
- `result=skipped`：条件更新未命中，通常是重复消息或非法状态流转。
- `result=error`：数据库更新出错。

## product-service

`stock_deduct_attempt_total`

product-service 每次开始处理库存扣减消息时递增，包括主队列和 retry queue 的处理尝试。

`stock_deduct_success_total`

库存扣减成功、库存结果成功消息发布完成、扣减日志更新为 `success` 后递增。

`stock_deduct_failed_total{reason}`

库存扣减最终失败时递增。

- `reason=insufficient stock`：业务库存不足。
- `reason=stock update skipped`：MySQL 条件扣减未命中。
- `reason=stock deduction retry exhausted`：临时异常超过最大重试次数后进入 dead queue。
- 其他 reason 对应代码中的库存处理失败原因。

## MQ

order-service 和 product-service 都暴露 MQ 消费指标。product-service 额外暴露 retry/dead 指标。

`mq_consume_total{queue}`

从 RabbitMQ 队列取到消息后递增。

`mq_consume_failed_total{queue}`

消息解析失败、业务处理返回可重试错误、retry/dead 路由失败或处理后需要 nack 时递增。

`mq_retry_total{queue}`

product-service 把库存扣减消息发布到 retry queue 后递增。当前 queue 标签值为 `stock.deduct.retry.queue`。

`mq_dead_total{queue}`

product-service 把库存扣减消息发布到 dead queue 后递增。当前 queue 标签值为 `stock.deduct.dead.queue`。

## 验证方式

启动服务后访问：

```powershell
curl http://localhost:9103/metrics
curl http://localhost:9102/metrics
```

创建订单后，可查看：

```powershell
curl http://localhost:9103/metrics | Select-String "orders_created_total"
```

库存扣减成功后，可查看：

```powershell
curl http://localhost:9102/metrics | Select-String "stock_deduct_success_total"
```

模拟 Redis/MySQL/RabbitMQ 临时异常并触发库存扣减消息，可观察：

```powershell
curl http://localhost:9102/metrics | Select-String "mq_retry_total|mq_dead_total|mq_consume_failed_total"
```
