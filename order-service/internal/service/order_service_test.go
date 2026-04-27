package service

import (
	"context"
	"errors"
	"testing"

	"order-service/internal/model"
	"order-service/internal/repository"
)

type stubOrderRepository struct {
	createFn       func(ctx context.Context, order *model.Order) error
	findByOrderID  func(ctx context.Context, orderID string) (*model.Order, error)
	updateStatusFn func(ctx context.Context, orderID string, status string, message string) error
}

func (s *stubOrderRepository) Create(ctx context.Context, order *model.Order) error {
	return s.createFn(ctx, order)
}

func (s *stubOrderRepository) FindByOrderID(ctx context.Context, orderID string) (*model.Order, error) {
	return s.findByOrderID(ctx, orderID)
}

func (s *stubOrderRepository) UpdateStatus(ctx context.Context, orderID string, status string, message string) error {
	return s.updateStatusFn(ctx, orderID, status, message)
}

type stubStockPublisher struct {
	publishFn func(ctx context.Context, msg StockDeduction) error
}

func (s *stubStockPublisher) PublishStockDeduction(ctx context.Context, msg StockDeduction) error {
	return s.publishFn(ctx, msg)
}

func TestCreateOrderInitialStatus(t *testing.T) {
	var savedOrder *model.Order
	repo := &stubOrderRepository{
		createFn: func(ctx context.Context, order *model.Order) error {
			savedOrder = &model.Order{
				OrderID:   order.OrderID,
				UserID:    order.UserID,
				ProductID: order.ProductID,
				Status:    order.Status,
				Message:   order.Message,
			}
			return nil
		},
		updateStatusFn: func(ctx context.Context, orderID string, status string, message string) error {
			return nil
		},
	}
	publisher := &stubStockPublisher{
		publishFn: func(ctx context.Context, msg StockDeduction) error { return nil },
	}

	svc := NewOrderService(repo, publisher)
	order, err := svc.CreateOrder(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if order.Status != model.OrderStatusPendingStock {
		t.Fatalf("expected status %s, got %s", model.OrderStatusPendingStock, order.Status)
	}
	if savedOrder == nil {
		t.Fatal("expected repository Create to be called")
	}
	if savedOrder.Status != model.OrderStatusPendingStock {
		t.Fatalf("expected saved order status %s, got %s", model.OrderStatusPendingStock, savedOrder.Status)
	}
}

func TestApplyStockResultSuccess(t *testing.T) {
	var updatedStatus string
	var updatedMessage string
	repo := &stubOrderRepository{
		createFn: func(ctx context.Context, order *model.Order) error { return nil },
		updateStatusFn: func(ctx context.Context, orderID string, status string, message string) error {
			updatedStatus = status
			updatedMessage = message
			return nil
		},
	}

	svc := NewOrderService(repo, &stubStockPublisher{})
	err := svc.ApplyStockResult(context.Background(), StockResult{OrderID: "ORD-1", Success: true})
	if err != nil {
		t.Fatalf("ApplyStockResult returned error: %v", err)
	}
	if updatedStatus != model.OrderStatusConfirmed {
		t.Fatalf("expected status %s, got %s", model.OrderStatusConfirmed, updatedStatus)
	}
	if updatedMessage != model.OrderMessageConfirmed {
		t.Fatalf("expected message %q, got %q", model.OrderMessageConfirmed, updatedMessage)
	}
}

func TestApplyStockResultFailure(t *testing.T) {
	var updatedStatus string
	var updatedMessage string
	repo := &stubOrderRepository{
		createFn: func(ctx context.Context, order *model.Order) error { return nil },
		updateStatusFn: func(ctx context.Context, orderID string, status string, message string) error {
			updatedStatus = status
			updatedMessage = message
			return nil
		},
	}

	svc := NewOrderService(repo, &stubStockPublisher{})
	err := svc.ApplyStockResult(context.Background(), StockResult{OrderID: "ORD-1", Success: false, Reason: "insufficient stock"})
	if err != nil {
		t.Fatalf("ApplyStockResult returned error: %v", err)
	}
	if updatedStatus != model.OrderStatusFailed {
		t.Fatalf("expected status %s, got %s", model.OrderStatusFailed, updatedStatus)
	}
	if updatedMessage != "insufficient stock" {
		t.Fatalf("expected custom reason, got %q", updatedMessage)
	}
}

func TestGetOrderNotFound(t *testing.T) {
	repo := &stubOrderRepository{
		createFn: func(ctx context.Context, order *model.Order) error { return nil },
		findByOrderID: func(ctx context.Context, orderID string) (*model.Order, error) {
			return nil, repository.ErrOrderNotFound
		},
		updateStatusFn: func(ctx context.Context, orderID string, status string, message string) error { return nil },
	}

	svc := NewOrderService(repo, &stubStockPublisher{})
	_, err := svc.GetOrder(context.Background(), "missing")
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}
