# 面试讲解提纲

## 30 秒版本

这是一个 Go 订单库存异步处理系统。网关用 Gin 对外提供 HTTP 接口，内部通过 gRPC 调用 user、product、order 服务。用户下单后，订单服务先创建 `pending_stock` 状态的订单，再通过 RabbitMQ 发送库存扣减消息。商品服务消费消息后，用 Redis 幂等 key 防止重复处理，用 Redis Lua 对库存缓存做原子预扣，再用 MySQL 条件更新扣减真实库存，最后把库存结果回写给订单服务，订单更新为 `confirmed` 或 `failed`。

## 核心链路

```text
登录拿 JWT
  -> 查询商品
  -> 创建 pending_stock 订单
  -> RabbitMQ stock_queue
  -> 商品服务异步扣库存
  -> RabbitMQ stock_result_queue
  -> 订单状态 confirmed / failed
```

## 两个重点

### 1. 订单和库存解耦

订单服务不直接调用商品服务扣库存，只发布库存扣减消息。商品服务独立消费消息并处理库存，处理完成后发布库存结果。这样订单服务主要关注订单生命周期，商品服务主要关注商品和库存处理。

### 2. 库存处理可控

商品服务处理库存消息时先检查 `processed_order:<order_id>`，避免 RabbitMQ 重复投递导致重复扣减。然后使用 Redis Lua 对 `product_stock:<product_id>` 做原子预扣，预扣成功后再通过 MySQL 条件更新作为库存扣减兜底。

## 可以主动说明的取舍

这个项目刻意没有继续堆服务治理组件，而是聚焦订单和库存这条主链路。当前重点是把 HTTP 接入、gRPC 调用、Redis 缓存、RabbitMQ 异步消息、JWT 鉴权和订单状态流转做清楚。
