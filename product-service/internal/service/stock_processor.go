package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"product-service/internal/metrics"
	"product-service/internal/model"
	"product-service/internal/repository"
	"product-service/internal/stock"

	"github.com/redis/go-redis/v9"
)

// stockDeductionQuantity 每次订单固定扣减 1 件库存
const stockDeductionQuantity int32 = 1

// StockDeduction 库存扣减消息结构
type StockDeduction struct {
	OrderID    string
	UserID     int32
	ProductID  int32
	RetryCount int32
}

// StockResultPublisher 库存扣减结果发布接口
type StockResultPublisher interface {
	PublishStockResult(ctx context.Context, msg StockDeduction, success bool, reason string) error
}

// stockProcessorRepository 库存处理所需的数据访问接口
type stockProcessorRepository interface {
	FindByID(ctx context.Context, productID int32) (*model.Product, error)
	DeductStockAndMarkLog(ctx context.Context, productID int32, quantity int32, orderID string, reason string) (bool, error)
	CreateStockDeductionLog(ctx context.Context, log *model.StockDeductionLog) (bool, error)
	FindStockDeductionLog(ctx context.Context, orderID string) (*model.StockDeductionLog, error)
	UpdateStockDeductionLog(ctx context.Context, orderID string, status string, reason string) error
}

// StockProcessor 库存扣减处理器：Redis Lua 原子预扣 + MySQL 事务扣减 + 幂等保障
type StockProcessor struct {
	repo      stockProcessorRepository
	rdb       *redis.Client
	publisher StockResultPublisher
}

// NewStockProcessor 创建库存处理器实例
func NewStockProcessor(
	repo stockProcessorRepository,
	rdb *redis.Client,
	publisher StockResultPublisher,
) *StockProcessor {
	return &StockProcessor{
		repo:      repo,
		rdb:       rdb,
		publisher: publisher,
	}
}

// Process 处理库存扣减请求，核心处理流程：
// 1. 创建/查询扣减日志（MySQL unique 幂等）
// 2. 检查 Redis 幂等键（快速去重）
// 3. Redis Lua 原子扣减 → MySQL 事务扣减 → 发布结果
func (p *StockProcessor) Process(ctx context.Context, msg StockDeduction) (bool, error) {
	metrics.StockDeductAttemptTotal.Inc()

	logEntry, shouldProcess, err := p.prepareStockDeductionLog(ctx, msg)
	if err != nil || !shouldProcess {
		return !shouldProcess, err
	}

	idempotentKey := stock.IdempotencyKey(msg.OrderID)
	if isBusinessFailureReason(logEntry.Reason) {
		return p.publishFailedStockResult(ctx, msg, idempotentKey, logEntry.Reason)
	}
	if logEntry.Reason == stock.ResultReasonMySQLDeducted {
		return p.publishSuccessfulStockResult(ctx, msg, idempotentKey)
	}
	if logEntry.Reason == stock.ResultReasonRedisDeducted {
		return p.handleRedisStockDeducted(ctx, msg, idempotentKey)
	}

	processed, err := p.rdb.Exists(ctx, idempotentKey).Result()
	if err != nil {
		return p.retryableError(msg, stock.ResultReasonRedisFailed, err)
	}
	if processed > 0 {
		if err := p.repo.UpdateStockDeductionLog(ctx, msg.OrderID, model.StockDeductionStatusSkipped, "redis_idempotency_key_exists"); err != nil {
			return p.retryableError(msg, stock.ResultReasonMySQLFailed, err)
		}
		log.Printf(
			"duplicate order ignored: order_id=%s product_id=%d retry_count=%d reason=%s",
			msg.OrderID,
			msg.ProductID,
			msg.RetryCount,
			"redis_idempotency_key_exists",
		)
		return true, nil
	}

	return p.deductStock(ctx, msg, idempotentKey, true)
}

// MarkRetryExhausted 标记扣减重试已耗尽，将日志状态设为失败
func (p *StockProcessor) MarkRetryExhausted(ctx context.Context, msg StockDeduction, reason string) error {
	finalReason := fmt.Sprintf("%s: %s", stock.ResultReasonRetryExhausted, reason)
	metrics.StockDeductFailedTotal.WithLabelValues(stock.ResultReasonRetryExhausted).Inc()
	return p.repo.UpdateStockDeductionLog(ctx, msg.OrderID, model.StockDeductionStatusFailed, finalReason)
}

// prepareStockDeductionLog 准备库存扣减日志：
// 首次创建成功返回 true；唯一冲突时查询已有记录，区分初次重试 vs 重复消费
func (p *StockProcessor) prepareStockDeductionLog(ctx context.Context, msg StockDeduction) (*model.StockDeductionLog, bool, error) {
	logEntry := &model.StockDeductionLog{
		OrderID:   msg.OrderID,
		ProductID: msg.ProductID,
		Quantity:  stockDeductionQuantity,
		Status:    model.StockDeductionStatusProcessing,
		Reason:    stock.ResultReasonStarted,
	}
	created, err := p.repo.CreateStockDeductionLog(ctx, logEntry)
	if err != nil {
		log.Printf(
			"create stock deduction log failed: order_id=%s product_id=%d retry_count=%d error=%v next_action=%s",
			msg.OrderID,
			msg.ProductID,
			msg.RetryCount,
			err,
			"retry",
		)
		return nil, false, err
	}
	if created {
		return logEntry, true, nil
	}

	existing, err := p.repo.FindStockDeductionLog(ctx, msg.OrderID)
	if err != nil {
		if errors.Is(err, repository.ErrStockDeductionLogNotFound) {
			return nil, false, fmt.Errorf("stock deduction log missing after unique conflict: order_id=%s", msg.OrderID)
		}
		return nil, false, err
	}
	if msg.RetryCount == 0 || existing.Status != model.StockDeductionStatusProcessing {
		log.Printf(
			"duplicate stock deduction ignored: order_id=%s product_id=%d retry_count=%d reason=%s",
			msg.OrderID,
			msg.ProductID,
			msg.RetryCount,
			"mysql_unique_order_id_conflict",
		)
		return existing, false, nil
	}

	log.Printf(
		"retry stock deduction resumed: order_id=%s product_id=%d retry_count=%d reason=%s",
		msg.OrderID,
		msg.ProductID,
		msg.RetryCount,
		existing.Reason,
	)
	return existing, true, nil
}

// deductStock 执行 Redis Lua 原子扣减脚本：
// 返回 1=成功, 0=库存不足, -1=缓存缺失
// 缓存缺失时尝试从 DB 回填缓存后重试一次
func (p *StockProcessor) deductStock(ctx context.Context, msg StockDeduction, idempotentKey string, allowReload bool) (bool, error) {
	cacheKey := stock.StockCacheKey(msg.ProductID)
	res, err := p.rdb.Eval(ctx, stock.LuaDeductStock, []string{cacheKey}, stockDeductionQuantity).Int()
	if err != nil {
		return p.retryableError(msg, stock.ResultReasonRedisFailed, err)
	}

	switch res {
	case stock.DeductSuccess:
		if err := p.repo.UpdateStockDeductionLog(ctx, msg.OrderID, model.StockDeductionStatusProcessing, stock.ResultReasonRedisDeducted); err != nil {
			return p.retryableError(msg, stock.ResultReasonMySQLFailed, err)
		}
		return p.handleRedisStockDeducted(ctx, msg, idempotentKey)
	case stock.DeductInsufficient:
		return p.publishFailedStockResult(ctx, msg, idempotentKey, stock.ResultReasonInsufficient)
	case stock.DeductCacheMissing:
		if allowReload && p.reloadStockCache(ctx, msg.ProductID, cacheKey) {
			return p.deductStock(ctx, msg, idempotentKey, false)
		}
		return p.retryableError(msg, stock.ResultReasonCacheMissing, nil)
	default:
		return p.retryableError(msg, "unexpected stock deduction result", fmt.Errorf("redis lua result=%d", res))
	}
}

// handleRedisStockDeducted Redis 扣减成功后，执行 MySQL 事务扣减并更新日志
func (p *StockProcessor) handleRedisStockDeducted(ctx context.Context, msg StockDeduction, idempotentKey string) (bool, error) {
	updated, err := p.repo.DeductStockAndMarkLog(ctx, msg.ProductID, stockDeductionQuantity, msg.OrderID, stock.ResultReasonMySQLDeducted)
	if err != nil {
		return p.retryableError(msg, stock.ResultReasonMySQLFailed, err)
	}
	if !updated {
		return p.publishFailedStockResult(ctx, msg, idempotentKey, stock.ResultReasonDBUpdateSkipped)
	}
	return p.publishSuccessfulStockResult(ctx, msg, idempotentKey)
}

// publishSuccessfulStockResult 发布扣减成功结果，删除商品缓存，写入 Redis 幂等键
func (p *StockProcessor) publishSuccessfulStockResult(ctx context.Context, msg StockDeduction, idempotentKey string) (bool, error) {
	_ = p.rdb.Del(ctx, stock.ProductCacheKey(msg.ProductID)).Err()
	if err := p.publisher.PublishStockResult(ctx, msg, true, stock.ResultReasonDeducted); err != nil {
		return p.retryableError(msg, "publish stock success result failed", err)
	}
	if err := p.repo.UpdateStockDeductionLog(ctx, msg.OrderID, model.StockDeductionStatusSuccess, stock.ResultReasonDeducted); err != nil {
		return p.retryableError(msg, stock.ResultReasonMySQLFailed, err)
	}
	_ = p.rdb.Set(ctx, idempotentKey, "1", 24*time.Hour).Err()
	metrics.StockDeductSuccessTotal.Inc()
	log.Printf(
		"stock decremented: order_id=%s product_id=%d retry_count=%d reason=%s",
		msg.OrderID,
		msg.ProductID,
		msg.RetryCount,
		stock.ResultReasonDeducted,
	)
	return true, nil
}

// publishFailedStockResult 发布扣减失败结果（库存不足等业务失败），写入 Redis 幂等键
func (p *StockProcessor) publishFailedStockResult(ctx context.Context, msg StockDeduction, idempotentKey string, reason string) (bool, error) {
	if err := p.repo.UpdateStockDeductionLog(ctx, msg.OrderID, model.StockDeductionStatusProcessing, reason); err != nil {
		return p.retryableError(msg, stock.ResultReasonMySQLFailed, err)
	}
	if err := p.publisher.PublishStockResult(ctx, msg, false, reason); err != nil {
		return p.retryableError(msg, "publish stock failed result failed", err)
	}
	if err := p.repo.UpdateStockDeductionLog(ctx, msg.OrderID, model.StockDeductionStatusFailed, reason); err != nil {
		return p.retryableError(msg, stock.ResultReasonMySQLFailed, err)
	}
	_ = p.rdb.Set(ctx, idempotentKey, "1", 24*time.Hour).Err()
	metrics.StockDeductFailedTotal.WithLabelValues(reason).Inc()
	log.Printf(
		"stock deduction business failed: order_id=%s product_id=%d retry_count=%d error=%s next_action=%s",
		msg.OrderID,
		msg.ProductID,
		msg.RetryCount,
		reason,
		"result_failed",
	)
	return true, nil
}

// retryableError 记录可重试错误日志，返回 false 触发消息 Nack/重试
func (p *StockProcessor) retryableError(msg StockDeduction, reason string, cause error) (bool, error) {
	if cause == nil {
		cause = errors.New(reason)
	}
	log.Printf(
		"stock deduction retryable error: order_id=%s product_id=%d retry_count=%d error=%v next_action=%s",
		msg.OrderID,
		msg.ProductID,
		msg.RetryCount,
		cause,
		"retry",
	)
	return false, fmt.Errorf("%s: %w", reason, cause)
}

// reloadStockCache 从数据库回填 Redis 库存缓存
func (p *StockProcessor) reloadStockCache(ctx context.Context, productID int32, cacheKey string) bool {
	product, err := p.repo.FindByID(ctx, productID)
	if err == nil {
		_ = p.rdb.Set(ctx, cacheKey, product.Stock, stock.StockCacheTTL).Err()
		return true
	}
	log.Printf("reload stock cache failed: product_id=%d reason=%s err=%v", productID, stock.ResultReasonCacheMissing, err)
	return false
}

// isBusinessFailureReason 判断是否为不可重试的业务失败原因
func isBusinessFailureReason(reason string) bool {
	return reason == stock.ResultReasonInsufficient || reason == stock.ResultReasonDBUpdateSkipped
}
