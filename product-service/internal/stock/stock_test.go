package stock

import "testing"

func TestStockKeys(t *testing.T) {
	if got := ProductCacheKey(1); got != "product:1" {
		t.Fatalf("unexpected product cache key: %s", got)
	}

	if got := StockCacheKey(1); got != "product_stock:1" {
		t.Fatalf("unexpected stock cache key: %s", got)
	}

	if got := IdempotencyKey("ORD-1"); got != "processed_order:ORD-1" {
		t.Fatalf("unexpected idempotency key: %s", got)
	}

	if got := ProductLockKey(1); got != "lock:product:1" {
		t.Fatalf("unexpected lock key: %s", got)
	}
}
