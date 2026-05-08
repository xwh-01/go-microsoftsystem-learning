# Go Order Flow

这是一个小型 Go 后端练习项目，目标不是堆技术栈，而是把一条订单链路讲清楚：

```text
注册/登录 -> 查询商品 -> 创建订单 -> 异步扣库存 -> 回写订单状态
```

## 项目重点

- 使用 Gin 提供 HTTP API。
- 使用 gRPC 连接内部服务。
- 使用 JWT 保护下单和订单查询接口。
- 使用 Redis 缓存商品信息和库存数据。
- 使用 RabbitMQ 解耦订单创建和库存扣减。
- 使用 Redis Lua 做库存缓存的原子预扣。
- 使用 Redis 幂等 key 降低重复消息导致重复扣库存的风险。

## 服务结构

```text
api-gateway      HTTP API, JWT, gRPC client
user-service     用户注册、登录、密码哈希
product-service  商品查询、商品缓存、库存扣减
order-service    订单创建、订单状态流转、库存结果消费
proto            gRPC 契约
```

## 核心下单流程

```text
POST /api/v1/auth/order
  -> api-gateway 校验 JWT
  -> order-service 创建 pending_stock 订单
  -> order-service 发布库存扣减消息到 stock_queue
  -> product-service 消费消息并处理库存
  -> product-service 发布库存结果到 stock_result_queue
  -> order-service 消费结果并更新订单为 confirmed 或 failed
```

订单状态只有三种：

```text
pending_stock -> confirmed
pending_stock -> failed
```

## 库存处理

商品服务消费库存消息后，会按下面顺序处理：

```text
1. 检查 processed_order:<order_id>，避免重复扣库存
2. 使用 Redis Lua 扣减 product_stock:<product_id>
3. Lua 返回 1：Redis 预扣成功，再用 MySQL 条件更新扣真实库存
4. Lua 返回 0：库存不足，发布失败结果
5. Lua 返回 -1：库存缓存不存在，从 MySQL 加载库存后让消息重试
6. 处理完成后发布库存结果给订单服务
```

## 本地运行

准备配置：

```powershell
Copy-Item .env.example .env
Copy-Item config/config.example.yaml config/config.yaml
```

启动基础设施：

```powershell
docker compose up -d redis rabbitmq mysql
```

按顺序启动服务：

```powershell
cd user-service
go run .
```

```powershell
cd product-service
go run .
```

```powershell
cd order-service
go run .
```

```powershell
cd api-gateway
go run .
```

## 接口示例

注册：

```powershell
curl -X POST http://localhost:8080/api/v1/public/register `
  -H "Content-Type: application/json" `
  -d "{\"username\":\"alice\",\"password\":\"123456\"}"
```

登录：

```powershell
curl -X POST http://localhost:8080/api/v1/public/login `
  -H "Content-Type: application/json" `
  -d "{\"username\":\"alice\",\"password\":\"123456\"}"
```

查询商品：

```powershell
curl http://localhost:8080/api/v1/public/product/1
```

创建订单：

```powershell
curl -X POST http://localhost:8080/api/v1/auth/order `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer <token>" `
  -d "{\"product_id\":1}"
```

查询订单：

```powershell
curl http://localhost:8080/api/v1/auth/order/<order_id> `
  -H "Authorization: Bearer <token>"
```

## 测试

```powershell
go test ./api-gateway/... ./user-service/... ./product-service/... ./order-service/... ./proto/...
```

库存处理链路有独立测试：

```powershell
go test ./product-service/internal/service -v
```
