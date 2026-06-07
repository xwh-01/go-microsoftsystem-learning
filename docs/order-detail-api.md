# 订单详情聚合接口

Gateway 新增订单详情接口：

```text
GET /api/v1/auth/orders/:id/detail
Authorization: Bearer <token>
```

该接口由 Gateway 做 JWT 鉴权，并聚合三个来源：

- order-service：查询订单基础信息和订单状态。
- product-service：查询商品信息。
- product-service `stock_deduction_logs`：查询库存扣减状态和失败原因。

用户只能查询自己的订单。如果订单的 `user_id` 和 JWT 中的用户 ID 不一致，Gateway 返回 `403`。

## 返回示例

```json
{
  "order_id": "ORD-1001",
  "user_id": 10,
  "status": "confirmed",
  "product": {
    "id": 1,
    "name": "iPhone 16 Pro",
    "price": 8999
  },
  "stock_result": {
    "status": "success",
    "reason": "stock deducted"
  },
  "created_at": "2026-06-07T10:00:00Z",
  "updated_at": "2026-06-07T10:00:01Z"
}
```

如果还没有查到 `stock_deduction_logs` 记录，`stock_result` 会返回：

```json
{
  "status": "pending",
  "reason": ""
}
```

## curl 示例

```powershell
curl http://localhost:8080/api/v1/auth/orders/<order_id>/detail `
  -H "Authorization: Bearer <token>"
```

常见响应：

- `200`：订单详情聚合成功。
- `401`：缺少或无效 token。
- `403`：尝试查询其他用户的订单。
- `404`：订单不存在，或订单关联商品不存在。
- `500`：内部服务查询失败。
