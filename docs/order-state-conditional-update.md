# 订单状态条件更新说明

order-service 消费 `stock_result_queue` 的库存结果消息时，订单状态更新必须带上当前状态条件：

```sql
UPDATE orders
SET status = 'confirmed'
WHERE order_id = ? AND status = 'pending_stock';

UPDATE orders
SET status = 'failed'
WHERE order_id = ? AND status = 'pending_stock';
```

本项目库存结果消息携带的是业务订单号 `order_id`；如果其他实现使用自增 `id` 定位订单，也必须在同一条 `UPDATE` 里同时带上 `status = 'pending_stock'` 条件。

原因是库存结果消息来自异步队列，RabbitMQ 可能出现重复投递、乱序到达或业务重试。`confirmed` 和 `failed` 都是订单最终状态，一旦订单已经进入最终状态，后到的重复消息或非法状态流转不能再覆盖它。

带上 `status = 'pending_stock'` 后，数据库会把状态机约束放进同一个原子更新里：

- `pending_stock -> confirmed` 可以成功。
- `pending_stock -> failed` 可以成功。
- `confirmed -> failed` 不会发生。
- `failed -> confirmed` 不会发生。

如果更新的 affected rows 为 0，说明订单已经不是 `pending_stock`，order-service 只记录日志并继续确认消息；这通常表示重复库存结果消息，或者某条消息试图执行非法状态流转。
