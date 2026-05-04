package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"product-service/internal/model"
	"product-service/internal/stock"
)

func TestStockProcessorDuplicateMessage(t *testing.T) {
	cache := &fakeStockCache{processed: true}
	repo := &fakeProductRepo{}
	publisher := &fakeStockPublisher{}
	processor := NewStockProcessor(repo, cache, publisher)

	ack, err := processor.Process(context.Background(), stockMsg())
	if err != nil {
		t.Fatalf("process duplicate message failed: %v", err)
	}
	if !ack {
		t.Fatalf("duplicate message should be acked")
	}
	if repo.deductCalls != 0 {
		t.Fatalf("duplicate message should not deduct mysql stock")
	}
	if len(publisher.results) != 0 {
		t.Fatalf("duplicate message should not publish stock result")
	}
}

func TestStockProcessorDeductSuccess(t *testing.T) {
	cache := &fakeStockCache{deductResult: stock.DeductSuccess}
	repo := &fakeProductRepo{deductUpdated: true}
	publisher := &fakeStockPublisher{}
	processor := NewStockProcessor(repo, cache, publisher)

	ack, err := processor.Process(context.Background(), stockMsg())
	if err != nil {
		t.Fatalf("process stock deduction failed: %v", err)
	}
	if !ack {
		t.Fatalf("successful stock deduction should be acked")
	}
	if repo.deductCalls != 1 {
		t.Fatalf("expected one mysql stock deduction, got %d", repo.deductCalls)
	}
	if !cache.productCacheDeleted {
		t.Fatalf("product cache should be deleted after stock changes")
	}
	if !cache.markedProcessed {
		t.Fatalf("order should be marked processed")
	}
	assertPublishedResult(t, publisher, true, stock.ResultReasonDeducted)
}

func TestStockProcessorInsufficientStock(t *testing.T) {
	cache := &fakeStockCache{deductResult: stock.DeductInsufficient}
	repo := &fakeProductRepo{}
	publisher := &fakeStockPublisher{}
	processor := NewStockProcessor(repo, cache, publisher)

	ack, err := processor.Process(context.Background(), stockMsg())
	if err != nil {
		t.Fatalf("process insufficient stock failed: %v", err)
	}
	if !ack {
		t.Fatalf("insufficient stock is a terminal result and should be acked")
	}
	if repo.deductCalls != 0 {
		t.Fatalf("insufficient redis stock should not deduct mysql stock")
	}
	if !cache.markedProcessed {
		t.Fatalf("terminal insufficient result should mark order processed")
	}
	assertPublishedResult(t, publisher, false, stock.ResultReasonInsufficient)
}

func TestStockProcessorCacheMissingReloadsStockAndRetries(t *testing.T) {
	cache := &fakeStockCache{deductResult: stock.DeductCacheMissing}
	repo := &fakeProductRepo{product: &model.Product{Stock: 8}}
	publisher := &fakeStockPublisher{}
	processor := NewStockProcessor(repo, cache, publisher)

	ack, err := processor.Process(context.Background(), stockMsg())
	if err != nil {
		t.Fatalf("process cache missing failed: %v", err)
	}
	if ack {
		t.Fatalf("cache missing should request message retry")
	}
	if cache.reloadedStock != 8 {
		t.Fatalf("expected stock cache to reload 8, got %d", cache.reloadedStock)
	}
	if len(publisher.results) != 0 {
		t.Fatalf("cache missing should not publish a terminal result")
	}
}

func TestStockProcessorDBUpdateSkipped(t *testing.T) {
	cache := &fakeStockCache{deductResult: stock.DeductSuccess}
	repo := &fakeProductRepo{deductUpdated: false}
	publisher := &fakeStockPublisher{}
	processor := NewStockProcessor(repo, cache, publisher)

	ack, err := processor.Process(context.Background(), stockMsg())
	if err != nil {
		t.Fatalf("process db update skipped failed: %v", err)
	}
	if !ack {
		t.Fatalf("db update skipped should be published as terminal result")
	}
	if cache.productCacheDeleted {
		t.Fatalf("product cache should not be deleted when mysql stock was not updated")
	}
	if !cache.markedProcessed {
		t.Fatalf("db update skipped should mark order processed")
	}
	assertPublishedResult(t, publisher, false, stock.ResultReasonDBUpdateSkipped)
}

func assertPublishedResult(t *testing.T, publisher *fakeStockPublisher, success bool, reason string) {
	t.Helper()
	if len(publisher.results) != 1 {
		t.Fatalf("expected one stock result, got %d", len(publisher.results))
	}
	got := publisher.results[0]
	if got.success != success || got.reason != reason {
		t.Fatalf("unexpected stock result: success=%v reason=%s", got.success, got.reason)
	}
}

func stockMsg() StockDeduction {
	return StockDeduction{
		OrderID:   "ORD-1",
		UserID:    1,
		ProductID: 1,
	}
}

type fakeProductRepo struct {
	product       *model.Product
	deductUpdated bool
	deductErr     error
	deductCalls   int
}

func (r *fakeProductRepo) Create(ctx context.Context, product *model.Product) error {
	return nil
}

func (r *fakeProductRepo) FindByID(ctx context.Context, productID int32) (*model.Product, error) {
	if r.product == nil {
		return nil, errors.New("product not found")
	}
	return r.product, nil
}

func (r *fakeProductRepo) DeductStock(ctx context.Context, productID int32, quantity int32) (bool, error) {
	r.deductCalls++
	return r.deductUpdated, r.deductErr
}

type fakeStockCache struct {
	processed           bool
	deductResult        int
	markedProcessed     bool
	productCacheDeleted bool
	reloadedStock       int32
}

func (c *fakeStockCache) IsOrderProcessed(ctx context.Context, key string) (bool, error) {
	return c.processed, nil
}

func (c *fakeStockCache) DeductCachedStock(ctx context.Context, key string, quantity int32) (int, error) {
	return c.deductResult, nil
}

func (c *fakeStockCache) MarkOrderProcessed(ctx context.Context, key string, ttl time.Duration) error {
	c.markedProcessed = true
	return nil
}

func (c *fakeStockCache) DeleteProductCache(ctx context.Context, productID int32) error {
	c.productCacheDeleted = true
	return nil
}

func (c *fakeStockCache) SetStock(ctx context.Context, key string, value int32, ttl time.Duration) error {
	c.reloadedStock = value
	return nil
}

type fakeStockPublisher struct {
	results []publishedStockResult
}

func (p *fakeStockPublisher) PublishStockResult(ctx context.Context, msg StockDeduction, success bool, reason string) error {
	p.results = append(p.results, publishedStockResult{
		success: success,
		reason:  reason,
	})
	return nil
}

type publishedStockResult struct {
	success bool
	reason  string
}
