package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"order-service/internal/model"
	"order-service/internal/repository"
)

type StockDeduction struct {
	OrderID   string `json:"order_id"`
	UserID    int32  `json:"user_id"`
	ProductID int32  `json:"product_id"`
}

type StockResult struct {
	OrderID   string `json:"order_id"`
	UserID    int32  `json:"user_id"`
	ProductID int32  `json:"product_id"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason"`
}

type StockPublisher interface {
	PublishStockDeduction(ctx context.Context, msg StockDeduction) error
}

type OrderService struct {
	repo      orderRepository
	publisher StockPublisher
}

type orderRepository interface {
	Create(ctx context.Context, order *model.Order) error
	FindByOrderID(ctx context.Context, orderID string) (*model.Order, error)
	UpdateStatus(ctx context.Context, orderID string, status string, message string) error
}

func NewOrderService(repo orderRepository, publisher StockPublisher) *OrderService {
	return &OrderService{
		repo:      repo,
		publisher: publisher,
	}
}

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

	msg := StockDeduction{
		OrderID:   order.OrderID,
		UserID:    order.UserID,
		ProductID: order.ProductID,
	}
	if err := s.publisher.PublishStockDeduction(ctx, msg); err != nil {
		_ = s.repo.UpdateStatus(ctx, order.OrderID, model.OrderStatusFailed, "publish stock message failed")
		return nil, fmt.Errorf("publish stock message failed: %w", err)
	}

	return order, nil
}

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

func (s *OrderService) ApplyStockResult(ctx context.Context, result StockResult) error {
	status := model.StatusFromStockResult(result.Success)
	message := model.MessageFromStockResult(result.Success, result.Reason)
	if err := s.repo.UpdateStatus(ctx, result.OrderID, status, message); err != nil {
		return fmt.Errorf("update order status failed: %w", err)
	}
	return nil
}

var ErrOrderNotFound = errors.New("order not found")
