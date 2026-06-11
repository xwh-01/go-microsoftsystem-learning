package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"order-service/internal/metrics"
	"order-service/internal/model"
	"order-service/internal/repository"
)

// StockDeduction 库存扣减请求消息结构
type StockDeduction struct {
	OrderID   string `json:"order_id"`
	UserID    int32  `json:"user_id"`
	ProductID int32  `json:"product_id"`
}

// StockResult 库存扣减结果消息结构
type StockResult struct {
	OrderID   string `json:"order_id"`
	UserID    int32  `json:"user_id"`
	ProductID int32  `json:"product_id"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason"`
}

// StockPublisher 库存扣减消息发布接口
type StockPublisher interface {
	PublishStockDeduction(ctx context.Context, msg StockDeduction) error
}

// OrderService 订单业务逻辑层
type OrderService struct {
	repo      orderRepository
	publisher StockPublisher
}

// orderRepository 订单仓储接口
type orderRepository interface {
	Create(ctx context.Context, order *model.Order) error
	FindByOrderID(ctx context.Context, orderID string) (*model.Order, error)
	UpdateStatus(ctx context.Context, orderID string, status string, message string) error
	UpdateStatusFrom(ctx context.Context, orderID string, expectedStatus string, targetStatus string, message string) (bool, error)
}

// NewOrderService 创建订单服务实例
func NewOrderService(repo orderRepository, publisher StockPublisher) *OrderService {
	return &OrderService{
		repo:      repo,
		publisher: publisher,
	}
}

// CreateOrder 创建订单：生成订单ID → 写入 pending_stock 状态 → 发布库存扣减消息
func (s *OrderService) CreateOrder(ctx context.Context, userID int32, productID int32) (*model.Order, error) {
	order := &model.Order{
		OrderID:   fmt.Sprintf("ORD-%d", time.Now().UnixNano()),
		UserID:    userID,
		ProductID: productID,
		Status:    model.OrderStatusPendingStock,
		Message:   model.OrderMessageWaitingStock,
	}

	if err := s.repo.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("create order record failed: %w", err)
	}
	metrics.OrdersCreatedTotal.Inc()

	msg := StockDeduction{
		OrderID:   order.OrderID,
		UserID:    order.UserID,
		ProductID: order.ProductID,
	}
	if err := s.publisher.PublishStockDeduction(ctx, msg); err != nil {
		_ = s.repo.UpdateStatus(ctx, order.OrderID, model.OrderStatusFailed, "publish stock message failed")
		metrics.OrdersFailedTotal.Inc()
		metrics.OrderStatusUpdateTotal.WithLabelValues(model.OrderStatusPendingStock, model.OrderStatusFailed, "applied").Inc()
		return nil, fmt.Errorf("publish stock message failed: %w", err)
	}

	return order, nil
}

// GetOrder 按订单 ID 查询订单
func (s *OrderService) GetOrder(ctx context.Context, orderID string) (*model.Order, error) {
	order, err := s.repo.FindByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("query order failed: %w", err)
	}
	return order, nil
}

// ApplyStockResult 处理库存扣减结果：条件更新订单状态（仅 pending_stock → confirmed/failed）
func (s *OrderService) ApplyStockResult(ctx context.Context, result StockResult) error {
	status := model.StatusFromStockResult(result.Success)
	message := model.MessageFromStockResult(result.Success, result.Reason)
	updated, err := s.repo.UpdateStatusFrom(ctx, result.OrderID, model.OrderStatusPendingStock, status, message)
	if err != nil {
		metrics.OrderStatusUpdateTotal.WithLabelValues(model.OrderStatusPendingStock, status, "error").Inc()
		return fmt.Errorf("update order status failed: %w", err)
	}
	if !updated {
		metrics.OrderStatusUpdateTotal.WithLabelValues(model.OrderStatusPendingStock, status, "skipped").Inc()
		log.Printf(
			"order status update skipped: order_id=%s expected_status=%s target_status=%s reason=%s",
			result.OrderID,
			model.OrderStatusPendingStock,
			status,
			"duplicate_message_or_illegal_state_transition",
		)
		return nil
	}

	metrics.OrderStatusUpdateTotal.WithLabelValues(model.OrderStatusPendingStock, status, "applied").Inc()
	if status == model.OrderStatusConfirmed {
		metrics.OrdersConfirmedTotal.Inc()
	}
	if status == model.OrderStatusFailed {
		metrics.OrdersFailedTotal.Inc()
	}

	log.Printf(
		"order status update applied: order_id=%s expected_status=%s target_status=%s reason=%s",
		result.OrderID,
		model.OrderStatusPendingStock,
		status,
		"stock_result_applied",
	)
	return nil
}

var ErrOrderNotFound = errors.New("order not found")
