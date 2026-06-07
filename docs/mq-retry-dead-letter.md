# RabbitMQ retry / dead queue 说明

product-service 保留原有主流程：

```text
stock_queue -> product-service -> stock_result_queue -> order-service
```

第三阶段新增两个库存扣减辅助队列：

```text
stock.deduct.retry.queue
stock.deduct.dead.queue
```

库存扣减消息新增 `retry_count` 字段。主队列消息没有该字段时按 `0` 处理。

## 哪些错误会 retry

以下错误属于临时异常，product-service 不直接丢弃消息，而是把消息重新发布到 `stock.deduct.retry.queue`：

- Redis 幂等 key 检查失败。
- Redis Lua 扣减执行失败。
- Redis 库存缓存缺失且本次无法从 MySQL 重新加载。
- MySQL 插入或查询 `stock_deduction_logs` 失败。
- MySQL 条件扣减执行异常。
- 发布库存结果到 `stock_result_queue` 失败。

retry 消息会带上原始 `order_id`、`product_id` 和递增后的 `retry_count`。

## 哪些错误会直接 failed

以下属于业务失败，不做无限重试，会直接向 order-service 发送库存失败结果：

- Redis Lua 判断库存不足。
- Redis 已预扣后，MySQL 条件扣减未命中，说明真实库存不足或扣减被跳过。

这类消息会更新 `stock_deduction_logs.status = failed`，并记录 `reason`，然后 ack 当前消息。日志里 `next_action=result_failed`。

## 什么时候进入 dead queue

当处理失败且 `retry_count < 3` 时：

```text
retry_count + 1 -> publish stock.deduct.retry.queue -> ack 当前消息
```

当处理失败且 `retry_count >= 3` 时：

```text
publish stock.deduct.dead.queue -> ack 当前消息
```

进入 dead queue 前会尽量把 `stock_deduction_logs.status` 更新为 `failed`，`reason` 记录为 `stock deduction retry exhausted: <error>`。如果当时 MySQL 也不可用，dead queue 消息本身仍会携带 `error` 字段，方便人工排查。

## 如何手动验证

1. 启动基础设施和服务。
2. 在 RabbitMQ 管理页面确认存在以下队列：
   - `stock_queue`
   - `stock.deduct.retry.queue`
   - `stock.deduct.dead.queue`
   - `stock_result_queue`
3. 临时停止 Redis 或配置错误 Redis 地址。
4. 向 `stock_queue` 发布库存扣减消息：

```json
{
  "order_id": "ORD-RETRY-1",
  "user_id": 1,
  "product_id": 1,
  "retry_count": 0
}
```

5. 查看 product-service 日志，应看到 `next_action=retry`，消息进入 `stock.deduct.retry.queue`。
6. 连续失败超过 3 次后，应看到 `next_action=dead`，消息进入 `stock.deduct.dead.queue`，消息体包含最后一次 `error`。
7. 恢复 Redis 后，再发布一条正常消息，应继续走原来的库存扣减和 `stock_result_queue` 主流程。

库存不足验证：把商品库存调为 0 或发布超过库存可扣的消息，应看到 `next_action=result_failed`，消息不会进入 retry/dead，而是直接发库存失败结果给 order-service。
