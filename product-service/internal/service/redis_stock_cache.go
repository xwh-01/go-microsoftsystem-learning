package service

import (
	"context"
	"time"

	"product-service/internal/stock"

	"github.com/redis/go-redis/v9"
)

type RedisStockCache struct {
	rdb *redis.Client
}

func NewRedisStockCache(rdb *redis.Client) *RedisStockCache {
	return &RedisStockCache{rdb: rdb}
}

func (c *RedisStockCache) IsOrderProcessed(ctx context.Context, key string) (bool, error) {
	processed, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return processed > 0, nil
}

func (c *RedisStockCache) DeductCachedStock(ctx context.Context, key string, quantity int32) (int, error) {
	return c.rdb.Eval(ctx, stock.LuaDeductStock, []string{key}, quantity).Int()
}

func (c *RedisStockCache) MarkOrderProcessed(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, "1", ttl).Err()
}

func (c *RedisStockCache) DeleteProductCache(ctx context.Context, productID int32) error {
	return c.rdb.Del(ctx, stock.ProductCacheKey(productID)).Err()
}

func (c *RedisStockCache) SetStock(ctx context.Context, key string, value int32, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}
