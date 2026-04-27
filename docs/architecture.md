# 架构说明

这份文档用于把项目的核心结构、时序和状态流转讲清楚。

## 1. 系统架构图

```text
Client
  |
  v
api-gateway
  |  \
  |   \-- gRPC -> user-service
  |   \-- gRPC -> product-service
  |   \-- gRPC -> order-service
  |
  v
Consul（服务发现）

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
user-service   -> SQLite
product-service -> MySQL + Redis + RabbitMQ
order-service   -> MySQL + RabbitMQ
api-gateway     -> Consul
```

## 2. 下单时序图

```text
用户
  -> api-gateway: POST /api/v1/auth/order
  -> api-gateway: 校验 JWT + 限流
  -> order-service: CreateOrder
  -> MySQL: 保存订单，状态 pending_stock
  -> RabbitMQ(stock_queue): 发布扣库存消息
  -> api-gateway: 返回 order_id

RabbitMQ(stock_queue)
  -> product-service: 消费扣库存消息
  -> Redis Lua: 原子扣减库存
  -> MySQL: 条件更新 stock > 0
  -> RabbitMQ(stock_result_queue): 发布库存结果

RabbitMQ(stock_result_queue)
  -> order-service: 消费库存结果
  -> MySQL: 更新订单状态 confirmed / failed

用户
  -> api-gateway: GET /api/v1/auth/order/:id
  -> order-service: GetOrder
  -> 返回订单最终状态
```

## 3. 注册登录时序图

```text
用户
  -> api-gateway: POST /api/v1/public/register
  -> user-service: Register
  -> SQLite: 保存用户名和 bcrypt 哈希密码
  -> 返回 user_id

用户
  -> api-gateway: POST /api/v1/public/login
  -> user-service: Login
  -> SQLite: 查询用户
  -> bcrypt: 校验密码
  -> api-gateway: 生成 JWT
  -> 返回 token
```

## 4. 商品查询路径

商品查询不是简单的“Redis 没有就查 MySQL”，而是多了一层缓存保护：

```text
请求商品 ID
  -> Bloom Filter 判断商品 ID 是否可能存在
  -> Redis 商品缓存
  -> Redis 空值缓存
  -> MySQL 查询
  -> 回填 Redis 缓存
```

如果商品不存在：

```text
MySQL 返回不存在
  -> Redis 写入短 TTL 空值缓存
  -> 后续相同请求直接被空值缓存挡住
```

## 5. 订单状态机

```text
pending_stock -> confirmed
pending_stock -> failed
```

状态说明：

| 状态 | 含义 |
| --- | --- |
| pending_stock | 订单已经创建，但库存还没有最终处理完成 |
| confirmed | 库存扣减成功，订单确认 |
| failed | 库存不足或库存处理失败 |

这个状态机的意义是：

```text
订单创建成功 != 订单业务最终成功
```

因为库存扣减是异步的，所以需要通过状态机把中间态和最终态表达清楚。

## 6. 为什么要这样分层

项目里的分层主要是为了把不同职责分开：

```text
main.go
  -> 启动和组装

handler / grpc / httpapi
  -> 协议转换

service
  -> 业务规则和状态流转

repository
  -> 数据库访问

model
  -> 核心数据结构和状态定义

mq / auth / discovery / stock
  -> 横向基础能力
```

这样读代码时可以按下面顺序看：

```text
请求先进入哪里
  -> 调用哪个 service
  -> service 又调用哪个 repository / mq / cache
```

## 7. 面试时可以如何总结

可以用下面这段总结整个系统：

```text
这是一个围绕订单状态闭环设计的 Go 微服务项目。
订单创建后不会同步扣库存，而是先进入 pending_stock，再通过 RabbitMQ 异步通知商品服务处理库存。
商品服务使用 Redis Lua 和 MySQL 条件更新来防止超卖，并通过库存结果队列把最终结果回传给订单服务。
订单服务据此把订单更新为 confirmed 或 failed，用户可以查询最终状态。
此外，商品查询加入了 Bloom Filter 和空值缓存，减少缓存穿透带来的数据库压力。
```
