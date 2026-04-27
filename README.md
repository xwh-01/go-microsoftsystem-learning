# Go 微服务订单系统

这是一个围绕“订单状态闭环”设计的小型 Go 微服务项目。  
项目的核心目标不是堆很多技术，而是把一条完整业务链路讲清楚：

```text
用户登录
  -> 查询商品
  -> 创建订单
  -> 订单进入 pending_stock
  -> 商品服务异步扣库存
  -> 订单更新为 confirmed 或 failed
  -> 用户查询最终订单状态
```

## 项目目标

这个项目重点体现三件事：

```text
1. 微服务之间如何协作
2. 异步扣库存如何形成订单状态闭环
3. 如何用缓存、消息队列、幂等和服务发现支撑这条业务主线
```

## 系统架构

```text
Client
  -> api-gateway (:8080, Gin/JWT/Sentinel/Consul Discovery)
  -> user-service (:50051, gRPC, SQLite, Consul)
  -> product-service (:50052, gRPC, MySQL/Redis/RabbitMQ/Consul)
  -> order-service (:50053, gRPC, MySQL/RabbitMQ/Consul)
```

## 核心业务流程

```text
POST /api/v1/auth/order
  -> api-gateway 校验 JWT，并做基础限流
  -> order-service 创建一条状态为 pending_stock 的订单
  -> order-service 向 RabbitMQ 发送扣库存消息
  -> product-service 消费消息并扣减库存
  -> product-service 把库存结果发送回 RabbitMQ
  -> order-service 消费结果并把订单更新为 confirmed 或 failed
```

## 订单状态机

```text
pending_stock -> confirmed
pending_stock -> failed
```

状态说明：

| 状态 | 含义 |
| --- | --- |
| pending_stock | 订单已经创建，正在等待库存扣减结果 |
| confirmed | 库存扣减成功，订单确认 |
| failed | 库存不足或库存处理失败 |

之所以需要 `pending_stock`，是因为库存扣减是异步完成的。  
创建订单成功，不代表库存已经扣减成功，所以需要一个中间状态来表示“订单已经接收，但业务还没最终完成”。

## 各服务职责

| 服务 | 职责 |
| --- | --- |
| api-gateway | 对外提供 HTTP API，处理 JWT、限流、服务发现、gRPC 转发 |
| user-service | 用户注册、登录、密码哈希存储 |
| product-service | 商品查询、商品缓存、缓存穿透防护、库存扣减、库存结果回传 |
| order-service | 订单创建、订单持久化、订单状态更新、订单查询 |
| proto | 微服务之间共享的 gRPC 契约 |

## 代码分层

项目保持“小而清晰”的原则，但核心服务已经做了轻量分层：

```text
api-gateway
  main.go
  internal/auth
  internal/discovery
  internal/httpapi

user-service
  main.go
  internal/model
  internal/repository
  internal/service
  internal/grpc

order-service
  main.go
  internal/model
  internal/repository
  internal/service
  internal/mq

product-service
  main.go
  internal/model
  internal/stock
  internal/mq
```

分层的意义是：

```text
main.go：负责启动和组装
handler / grpc / httpapi：负责协议转换
service：负责业务规则和状态流转
repository：负责数据库访问
model：负责核心数据结构和状态定义
mq / auth / discovery / stock：负责横向基础能力
```

## 为什么使用 RabbitMQ

订单服务不直接同步扣库存，而是先写订单，再发消息：

```text
order-service：负责订单生命周期
product-service：负责商品和库存
RabbitMQ：负责异步连接两者
```

这样做的好处：

```text
1. 下单接口返回更快
2. 订单服务和库存服务解耦
3. 可以通过订单状态表达“处理中”和“最终结果”
```

## 商品查询如何防缓存穿透

商品查询现在有三层保护：

```text
布隆过滤器
  -> 先过滤明显不存在的商品 ID

Redis 正常缓存
  -> 命中商品数据直接返回

Redis 空值缓存
  -> 对不存在商品写一个短 TTL 的空值标记
```

这样可以减少恶意或错误请求对 MySQL 的直接冲击。

## 库存扣减如何工作

库存扣减使用两层保护：

```text
Redis Lua
  -> 原子判断库存是否足够，并扣减缓存库存

MySQL 条件更新
  -> 用 WHERE stock > 0 做最终兜底
```

Redis Lua 返回三种结果：

| Lua 返回值 | 含义 | 商品服务处理方式 |
| --- | --- | --- |
| `1` | 缓存库存扣减成功 | 更新 MySQL，发布成功结果，记录幂等 key |
| `0` | 库存不足 | 发布失败结果，记录幂等 key |
| `-1` | Redis 中没有库存缓存 | 从 MySQL 重新加载库存，再让消息重试 |

## 幂等处理

RabbitMQ 可能出现重复投递，所以商品服务使用 Redis key：

```text
processed_order:<order_id>
```

如果这个 key 已经存在，说明该订单的库存消息已经处理过，不会重复扣库存。

## 支撑能力

| 能力 | 作用 |
| --- | --- |
| Consul | 服务注册发现，网关通过服务名找到内部服务地址 |
| Sentinel | 对下单接口做基础限流 |
| Redis 缓存 | 缓存商品数据和库存数据 |
| Redis 空值缓存 + Bloom Filter | 防缓存穿透 |

## 如何运行

### 1. 准备本地配置

```powershell
Copy-Item .env.example .env
Copy-Item config/config.example.yaml config/config.yaml
```

请在 `.env` 和 `config/config.yaml` 中填入本机数据库、RabbitMQ 和 JWT 密钥。`config/config.yaml`、`.env`、本地数据库文件会被 Git 忽略，不会上传。

### 2. 启动基础设施

在项目根目录执行：

```powershell
docker compose up -d redis consul rabbitmq mysql
```

其中：

```text
RabbitMQ 管理台：http://localhost:15672
账号密码：使用 .env 中的 RABBITMQ_DEFAULT_USER / RABBITMQ_DEFAULT_PASS

Consul 管理台：http://localhost:8500
```

### 3. 启动微服务

请按这个顺序启动：

```text
user-service -> product-service -> order-service -> api-gateway
```

分别打开 4 个终端执行：

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

说明：

```text
api-gateway 启动时会从 Consul 发现 user-service、product-service、order-service，
所以必须先让内部服务完成注册，再启动网关。
```

## 接口示例

### 注册

```powershell
curl -X POST http://localhost:8080/api/v1/public/register `
  -H "Content-Type: application/json" `
  -d "{\"username\":\"alice\",\"password\":\"123456\"}"
```

### 登录

```powershell
curl -X POST http://localhost:8080/api/v1/public/login `
  -H "Content-Type: application/json" `
  -d "{\"username\":\"alice\",\"password\":\"123456\"}"
```

登录成功后会返回 `token`。

### 查询商品

```powershell
curl http://localhost:8080/api/v1/public/product/1
```

### 创建订单

```powershell
curl -X POST http://localhost:8080/api/v1/auth/order `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer <token>" `
  -d "{\"product_id\":1}"
```

### 查询订单

```powershell
curl http://localhost:8080/api/v1/auth/order/<order_id> `
  -H "Authorization: Bearer <token>"
```

## 如何验证项目

你至少可以按下面顺序手动验证一遍：

```text
1. 注册一个用户
2. 登录拿到 token
3. 查询商品 1
4. 创建订单
5. 用返回的 order_id 查询订单状态
```

正常情况下：

```text
刚创建订单时，订单会先进入 pending_stock
库存消息处理完成后，订单会变成 confirmed 或 failed
```

## 测试命令

项目根目录是 Go workspace，推荐在根目录执行：

```powershell
go test ./api-gateway/... ./user-service/... ./product-service/... ./order-service/... ./proto/...
```

如果 Windows 的默认 Go build cache 有权限问题，可以临时改成本地缓存：

```powershell
$env:GOCACHE=(Join-Path (Get-Location) '.gocache')
go test ./api-gateway/... ./user-service/... ./product-service/... ./order-service/... ./proto/...
```

## 面试怎么讲

这个项目最适合这样讲：

```text
这是一个 Go 微服务订单系统。
用户登录后可以查询商品并创建订单。
订单服务先创建 pending_stock 状态订单，再通过 RabbitMQ 异步通知商品服务扣库存。
商品服务使用 Redis Lua 做原子库存预扣，并用 MySQL 条件更新做最终兜底。
库存结果通过另一个消息队列回传给订单服务，订单服务最终把订单更新为 confirmed 或 failed。
此外，商品查询还加入了 Bloom Filter 和空值缓存来防止缓存穿透。
```
