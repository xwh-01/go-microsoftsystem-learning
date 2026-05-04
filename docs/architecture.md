# 架构说明

这份文档只描述项目的核心链路，方便复习和面试讲解。

## 系统结构

```text
Client
  |
  v
api-gateway
  |-- gRPC -> user-service
  |-- gRPC -> product-service
  |-- gRPC -> order-service

order-service
  |
  v
RabbitMQ stock_queue
  |
  v
product-service
  |
  v
RabbitMQ stock_result_queue
  |
  v
order-service
```

基础设施：

```text
user-service    -> MySQL
product-service -> MySQL + Redis + RabbitMQ
order-service   -> MySQL + RabbitMQ
api-gateway     -> Gin + JWT + gRPC client
```

## 注册登录

```text
POST /api/v1/public/register
  -> api-gateway
  -> user-service.Register
  -> bcrypt 生成密码哈希
  -> MySQL 保存用户

POST /api/v1/public/login
  -> api-gateway
  -> user-service.Login
  -> MySQL 查询用户
  -> bcrypt 校验密码
  -> api-gateway 生成 JWT
```

## 商品查询

```text
GET /api/v1/public/product/:id
  -> api-gateway
  -> product-service.GetProduct
  -> 先查 Redis 商品缓存 product:<id>
  -> 缓存未命中时查 MySQL
  -> 商品不存在时写入短 TTL 空值缓存
  -> 商品存在时写回 Redis，并同步库存缓存 product_stock:<id>
```

## 创建订单

```text
POST /api/v1/auth/order
  -> api-gateway 校验 JWT
  -> order-service.CreateOrder
  -> MySQL 保存 pending_stock 订单
  -> RabbitMQ stock_queue 发布库存扣减消息
  -> 返回 order_id
```

订单创建成功只表示订单被接收，不表示库存已经扣减成功。

## 异步扣库存

```text
product-service 消费 stock_queue
  -> 检查 processed_order:<order_id>
  -> 执行 Redis Lua 扣 product_stock:<product_id>
  -> Redis 预扣成功后执行 MySQL 条件更新
  -> 发布库存结果到 stock_result_queue
```

Lua 返回值：

```text
1   Redis 预扣成功，继续扣 MySQL
0   Redis 库存不足，发布失败结果
-1  Redis 库存缓存不存在，重新加载缓存后让消息重试
```

MySQL 条件更新：

```sql
UPDATE products
SET stock = stock - 1
WHERE id = ? AND stock >= 1;
```

## 订单状态回写

```text
order-service 消费 stock_result_queue
  -> success=true  更新订单为 confirmed
  -> success=false 更新订单为 failed
```

状态机：

```text
pending_stock -> confirmed
pending_stock -> failed
```

## 面试讲法

这个项目可以概括成：

```text
我用 Go 做了一个订单交易链路练习项目。网关用 Gin 对外提供 HTTP 接口，内部通过 gRPC 调用用户、商品和订单服务。下单时订单服务先创建 pending_stock 状态的订单，然后通过 RabbitMQ 发送扣库存消息。商品服务消费消息后，先用 Redis 幂等 key 判断是否重复处理，再用 Redis Lua 对库存缓存做原子预扣，预扣成功后通过 MySQL 条件更新扣减真实库存，最后把库存处理结果发回订单服务，订单服务根据结果把订单更新成 confirmed 或 failed。
```
