package service

import (
	"context"
	"log"
	"time"

	"product-service/internal/stock"
)

type StockDeduction struct {
	OrderID   string
	UserID    int32
	ProductID int32
}

type StockResultPublisher interface {
	PublishStockResult(ctx context.Context, msg StockDeduction, success bool, reason string) error
}

type stockCache interface {
	IsOrderProcessed(ctx context.Context, key string) (bool, error)
	DeductCachedStock(ctx context.Context, key string, quantity int32) (int, error)
	MarkOrderProcessed(ctx context.Context, key string, ttl time.Duration) error
	DeleteProductCache(ctx context.Context, productID int32) error
	SetStock(ctx context.Context, key string, value int32, ttl time.Duration) error
}

type StockProcessor struct {
	repo      productRepository
	cache     stockCache
	publisher StockResultPublisher
}

func NewStockProcessor(
	repo productRepository,
	cache stockCache,
	publisher StockResultPublisher,
) *StockProcessor {
	return &StockProcessor{
		repo:      repo,
		cache:     cache,
		publisher: publisher,
	}
}

func (p *StockProcessor) Process(ctx context.Context, msg StockDeduction) (bool, error) {
	idempotentKey := stock.IdempotencyKey(msg.OrderID)
	processed, err := p.cache.IsOrderProcessed(ctx, idempotentKey)
	if err != nil {
		log.Printf("idempotency check failed: %v", err)
		return false, err
	}
	if processed {
		log.Printf("duplicate order ignored: %s", msg.OrderID)
		return true, nil
	}

	cacheKey := stock.StockCacheKey(msg.ProductID)
	res, err := p.cache.DeductCachedStock(ctx, cacheKey, 1)
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
		_ = p.cache.MarkOrderProcessed(ctx, idempotentKey, 24*time.Hour)
		return true, nil
	}

	_ = p.cache.DeleteProductCache(ctx, msg.ProductID)
	if err := p.publisher.PublishStockResult(ctx, msg, true, stock.ResultReasonDeducted); err != nil {
		log.Printf("publish stock result failed: %v", err)
		return false, err
	}

	_ = p.cache.MarkOrderProcessed(ctx, idempotentKey, 24*time.Hour)
	log.Printf("stock decremented for order %s", msg.OrderID)
	return true, nil
}

func (p *StockProcessor) handleInsufficientStock(ctx context.Context, msg StockDeduction, idempotentKey string) (bool, error) {
	log.Printf("insufficient stock: order_id=%s product_id=%d", msg.OrderID, msg.ProductID)
	if err := p.publisher.PublishStockResult(ctx, msg, false, stock.ResultReasonInsufficient); err != nil {
		log.Printf("publish stock result failed: %v", err)
		return false, err
	}
	_ = p.cache.MarkOrderProcessed(ctx, idempotentKey, 24*time.Hour)
	return true, nil
}

func (p *StockProcessor) reloadStockCache(ctx context.Context, productID int32, cacheKey string) {
	product, err := p.repo.FindByID(ctx, productID)
	if err == nil {
		_ = p.cache.SetStock(ctx, cacheKey, product.Stock, stock.StockCacheTTL)
	}
}
