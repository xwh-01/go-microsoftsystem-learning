package repository

import (
	"context"
	"testing"

	"order-service/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUpdateStatusFromOnlyUpdatesExpectedStatus(t *testing.T) {
	repo := newTestOrderRepository(t)
	ctx := context.Background()

	order := &model.Order{
		OrderID:   "ORD-1",
		UserID:    1,
		ProductID: 2,
		Status:    model.OrderStatusPendingStock,
		Message:   model.OrderMessageWaitingStock,
	}
	if err := repo.Create(ctx, order); err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	updated, err := repo.UpdateStatusFrom(ctx, order.OrderID, model.OrderStatusPendingStock, model.OrderStatusConfirmed, model.OrderMessageConfirmed)
	if err != nil {
		t.Fatalf("update status from pending failed: %v", err)
	}
	if !updated {
		t.Fatal("expected pending order to be updated")
	}

	saved, err := repo.FindByOrderID(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("find order failed: %v", err)
	}
	if saved.Status != model.OrderStatusConfirmed {
		t.Fatalf("expected status %s, got %s", model.OrderStatusConfirmed, saved.Status)
	}

	updated, err = repo.UpdateStatusFrom(ctx, order.OrderID, model.OrderStatusPendingStock, model.OrderStatusFailed, "late failure")
	if err != nil {
		t.Fatalf("late update failed: %v", err)
	}
	if updated {
		t.Fatal("expected confirmed order not to be overwritten by late stock result")
	}

	saved, err = repo.FindByOrderID(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("find order after late update failed: %v", err)
	}
	if saved.Status != model.OrderStatusConfirmed {
		t.Fatalf("expected final status to remain %s, got %s", model.OrderStatusConfirmed, saved.Status)
	}
}

func TestUpdateStatusFromLeavesFailedOrderFinal(t *testing.T) {
	repo := newTestOrderRepository(t)
	ctx := context.Background()

	order := &model.Order{
		OrderID:   "ORD-2",
		UserID:    1,
		ProductID: 2,
		Status:    model.OrderStatusFailed,
		Message:   model.OrderMessageFailed,
	}
	if err := repo.Create(ctx, order); err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	updated, err := repo.UpdateStatusFrom(ctx, order.OrderID, model.OrderStatusPendingStock, model.OrderStatusConfirmed, model.OrderMessageConfirmed)
	if err != nil {
		t.Fatalf("late success update failed: %v", err)
	}
	if updated {
		t.Fatal("expected failed order not to be overwritten by late stock result")
	}

	saved, err := repo.FindByOrderID(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("find order failed: %v", err)
	}
	if saved.Status != model.OrderStatusFailed {
		t.Fatalf("expected final status to remain %s, got %s", model.OrderStatusFailed, saved.Status)
	}
}

func TestUpdateStatusFromCanFailPendingOrder(t *testing.T) {
	repo := newTestOrderRepository(t)
	ctx := context.Background()

	order := &model.Order{
		OrderID:   "ORD-3",
		UserID:    1,
		ProductID: 2,
		Status:    model.OrderStatusPendingStock,
		Message:   model.OrderMessageWaitingStock,
	}
	if err := repo.Create(ctx, order); err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	updated, err := repo.UpdateStatusFrom(ctx, order.OrderID, model.OrderStatusPendingStock, model.OrderStatusFailed, "insufficient stock")
	if err != nil {
		t.Fatalf("update status from pending failed: %v", err)
	}
	if !updated {
		t.Fatal("expected pending order to be failed")
	}

	saved, err := repo.FindByOrderID(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("find order failed: %v", err)
	}
	if saved.Status != model.OrderStatusFailed {
		t.Fatalf("expected status %s, got %s", model.OrderStatusFailed, saved.Status)
	}
}

func newTestOrderRepository(t *testing.T) *OrderRepository {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}

	repo := NewOrderRepository(db)
	if err := repo.AutoMigrate(); err != nil {
		t.Fatalf("migrate order table failed: %v", err)
	}
	return repo
}
