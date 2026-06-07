# 库存扣减幂等说明

product-service 消费 `stock_queue` 时，会先写入 `stock_deduction_logs`：

```text
order_id   unique
product_id
quantity
status     processing / success / failed / skipped
reason
created_at
updated_at
```

## Redis 幂等 key 的作用

Redis key `processed_order:<order_id>` 用来快速识别已经处理过的订单消息，减少重复消息继续进入 Redis Lua 和 MySQL 扣减逻辑的机会。它适合做高性能的短期幂等保护，但 Redis key 有 TTL，也可能因为缓存淘汰、重启或运维操作而丢失。

## MySQL 唯一键为什么是最终兜底

`stock_deduction_logs.order_id` 有唯一约束。RabbitMQ 可能重复投递同一条库存扣减消息，如果只依赖 Redis key，Redis key 丢失后仍可能重复扣减库存。

MySQL 的 `unique(order_id)` 是持久化约束，和业务扣减使用同一个数据库系统保存状态。消息进入处理流程时先尝试插入 `processing` 日志，只有插入成功的消费者才能继续执行 Redis Lua 预扣和 MySQL 条件扣减。重复消息插入失败或被忽略时，说明该订单已经进入过库存扣减流程，系统直接记录日志并 ack 消息，不再重复扣库存。

## 重复消费如何处理

1. 消费者收到库存扣减消息。
2. 先尝试插入 `stock_deduction_logs(order_id, product_id, quantity, status)`，状态为 `processing`。
3. 如果 `order_id` 唯一键冲突，记录重复消费日志并 ack，不再执行 Redis Lua 或 MySQL 扣减。
4. 如果插入成功，继续执行 Redis Lua 预扣。
5. Redis 预扣成功后执行 MySQL 条件扣减，成功则把日志更新为 `success`。
6. 库存不足、MySQL 条件扣减未命中或其他扣减失败时，把日志更新为 `failed` 并记录 `reason`。

这样 Redis key 负责快速过滤重复消息，MySQL 唯一键负责在 Redis 失效或重复投递场景下提供最终幂等保护。
