package stock

import (
	"fmt"
	"time"
)

// Redis Lua 脚本返回值
const (
	DeductSuccess      = 1  // 扣减成功
	DeductInsufficient = 0  // 库存不足
	DeductCacheMissing = -1 // 缓存中无库存数据
)

// 库存扣减结果原因常量
const (
	ResultReasonStarted         = "stock deduction started"
	ResultReasonDeducted        = "stock deducted"
	ResultReasonRedisDeducted   = "redis stock deducted"
	ResultReasonMySQLDeducted   = "mysql stock deducted"
	ResultReasonInsufficient    = "insufficient stock"
	ResultReasonDBUpdateSkipped = "stock update skipped"
	ResultReasonCacheMissing    = "stock cache missing"
	ResultReasonRedisFailed     = "redis stock deduction failed"
	ResultReasonMySQLFailed     = "mysql stock deduction failed"
	ResultReasonRetryExhausted  = "stock deduction retry exhausted"
)

const (
	NullProductCacheValue = "__null_product__"
)

// 缓存 TTL 常量
const (
	ProductCacheTTL     = 10 * time.Minute
	NullProductCacheTTL = 1 * time.Minute
	StockCacheTTL       = 10 * time.Minute
)

// LuaDeductStock Redis Lua 原子扣减脚本：先检查库存是否充足，再执行 DECRBY
const LuaDeductStock = `
local stock = tonumber(redis.call('get', KEYS[1]))
if stock == nil then
    return -1
end
if stock >= tonumber(ARGV[1]) then
    redis.call('decrby', KEYS[1], ARGV[1])
    return 1
end
return 0
`

// ProductCacheKey 生成商品信息缓存键
func ProductCacheKey(productID int32) string {
	return fmt.Sprintf("product:%d", productID)
}

// StockCacheKey 生成库存数量缓存键
func StockCacheKey(productID int32) string {
	return fmt.Sprintf("product_stock:%d", productID)
}

// IdempotencyKey 生成订单幂等键（用于防止重复扣减）
func IdempotencyKey(orderID string) string {
	return fmt.Sprintf("processed_order:%s", orderID)
}
