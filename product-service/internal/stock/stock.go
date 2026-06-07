package stock

import (
	"fmt"
	"time"
)

const (
	DeductSuccess      = 1
	DeductInsufficient = 0
	DeductCacheMissing = -1
)

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

const (
	ProductCacheTTL     = 10 * time.Minute
	NullProductCacheTTL = 1 * time.Minute
	StockCacheTTL       = 10 * time.Minute
)

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

func ProductCacheKey(productID int32) string {
	return fmt.Sprintf("product:%d", productID)
}

func StockCacheKey(productID int32) string {
	return fmt.Sprintf("product_stock:%d", productID)
}

func IdempotencyKey(orderID string) string {
	return fmt.Sprintf("processed_order:%s", orderID)
}
