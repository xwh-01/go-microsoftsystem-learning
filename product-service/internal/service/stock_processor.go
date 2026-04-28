package service

import (
	"context"
	"log"
	"time"

	"product-service/internal/stock"

	"github.com/go-redsync/redsync/v4"
	"github.com/redis/go-redis/v9"
)

type StockDeduction struct {
	OrderID   string
	UserID    int32
	ProductID int32
}

type StockResultPublisher interface {
	PublishStockResult(ctx context.Context, msg StockDeduction, success bool, reason string) error
}

type StockProcessor struct {
	repo      productRepository
	rdb       *redis.Client
	locks     *redsync.Redsync
	publisher StockResultPublisher
}

func NewStockProcessor(
	repo productRepository,
	rdb *redis.Client,
	locks *redsync.Redsync,
	publisher StockResultPublisher,
) *StockProcessor {
	return &StockProcessor{
		repo:      repo,
		rdb:       rdb,
		locks:     locks,
		publisher: publisher,
	}
}

func (p *StockProcessor) Process(ctx context.Context, msg StockDeduction) (bool, error) {
	idempotentKey := stock.IdempotencyKey(msg.OrderID)
	processed, err := p.rdb.Exists(ctx, idempotentKey).Result()
	if err != nil {
		log.Printf("idempotency check failed: %v", err)
		return false, err
	}
	if processed > 0 {
		log.Printf("duplicate order ignored: %s", msg.OrderID)
		return true, nil
	}

	cacheKey := stock.StockCacheKey(msg.ProductID)
	res, err := p.rdb.Eval(ctx, stock.LuaDeductStock, []string{cacheKey}, 1).Int()
	if err != nil {
		log.Printf("decrement stock in redis failed: %v", err)
		return false, err
	}

	switch res {
	case stock.DeductSuccess:
		return p.handleRedisStockDeducted(ctx, msg, idempotentKey)
	case stock.DeductInsufficient:
		return p.handleInsufficientStock(ctx, msg, idempotentKey)
	case stock.DeductCacheMissing:
		p.reloadStockCache(ctx, msg.ProductID, cacheKey)
		return false, nil
	default:
		log.Printf("unexpected stock deduction result: %d", res)
		return false, nil
	}
}

func (p *StockProcessor) handleRedisStockDeducted(ctx context.Context, msg StockDeduction, idempotentKey string) (bool, error) {
	mutex := p.locks.NewMutex(stock.ProductLockKey(msg.ProductID))
	if err := mutex.Lock(); err != nil {
		log.Printf("acquire stock lock failed: %v", err)
		return false, err
	}
	defer func() {
		_, _ = mutex.Unlock()
	}()

	updated, err := p.repo.DeductStock(ctx, msg.ProductID, 1)
	if err != nil {
		log.Printf("update stock in mysql failed: %v", err)
		return false, err
	}

	if !updated {
		log.Printf("stock update skipped for order %s", msg.OrderID)
		if err := p.publisher.PublishStockResult(ctx, msg, false, stock.ResultReasonDBUpdateSkipped); err != nil {
			log.Printf("publish stock result failed: %v", err)
			return false, err
		}
		_ = p.rdb.Set(ctx, idempotentKey, "1", 24*time.Hour).Err()
		return true, nil
	}

	_ = p.rdb.Del(ctx, stock.ProductCacheKey(msg.ProductID)).Err()
	if err := p.publisher.PublishStockResult(ctx, msg, true, stock.ResultReasonDeducted); err != nil {
		log.Printf("publish stock result failed: %v", err)
		return false, err
	}

	_ = p.rdb.Set(ctx, idempotentKey, "1", 24*time.Hour).Err()
	log.Printf("stock decremented for order %s", msg.OrderID)
	return true, nil
}

func (p *StockProcessor) handleInsufficientStock(ctx context.Context, msg StockDeduction, idempotentKey string) (bool, error) {
	log.Printf("insufficient stock: order_id=%s product_id=%d", msg.OrderID, msg.ProductID)
	if err := p.publisher.PublishStockResult(ctx, msg, false, stock.ResultReasonInsufficient); err != nil {
		log.Printf("publish stock result failed: %v", err)
		return false, err
	}
	_ = p.rdb.Set(ctx, idempotentKey, "1", 24*time.Hour).Err()
	return true, nil
}

func (p *StockProcessor) reloadStockCache(ctx context.Context, productID int32, cacheKey string) {
	product, err := p.repo.FindByID(ctx, productID)
	if err == nil {
		_ = p.rdb.Set(ctx, cacheKey, product.Stock, stock.StockCacheTTL).Err()
	}
}
