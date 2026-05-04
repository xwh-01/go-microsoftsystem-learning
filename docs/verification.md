# 可验证产出

这个项目现在只保留一条核心链路：

```text
创建订单 -> 异步扣库存 -> 回写订单状态
```

为了让项目不是停留在“用了 Redis / RabbitMQ”的描述上，库存处理链路补了可运行的单元测试。

## 库存处理测试

测试文件：

```text
product-service/internal/service/stock_processor_test.go
```

覆盖场景：

| 测试场景 | 预期结果 |
| --- | --- |
| 重复订单消息 | 直接 ACK，不重复扣 MySQL，不重复发布库存结果 |
| Redis 预扣成功 | 扣 MySQL，删除商品缓存，发布成功结果，写入幂等 key |
| Redis 库存不足 | 不扣 MySQL，发布失败结果，写入幂等 key |
| Redis 库存缓存缺失 | 从 MySQL 重新加载库存，请求 RabbitMQ 重试 |
| MySQL 条件更新失败 | 发布失败结果，写入幂等 key，不删除商品缓存 |

运行命令：

```powershell
go test ./product-service/internal/service -v
```

当前结果：

```text
5 个库存处理核心场景全部通过
```

可以进一步查看覆盖率：

```powershell
go test ./product-service/internal/service -cover
```

## 全量测试

运行命令：

```powershell
go test ./api-gateway/... ./user-service/... ./product-service/... ./order-service/... ./proto/...
```

当前结果：

```text
全部模块测试通过
```

## 面试表述

可以这样讲：

```text
我没有继续扩展很多功能，而是把订单异步扣库存这条链路做成可验证产出。库存处理器通过接口隔离 Redis 缓存能力，测试中使用 fake cache / fake repository / fake publisher 验证核心分支，包括重复消费、库存不足、缓存缺失重试、MySQL 条件更新失败等场景。这样可以证明 RabbitMQ 重复投递和库存处理异常时，系统会走到预期分支，而不是只靠手工联调。
```
