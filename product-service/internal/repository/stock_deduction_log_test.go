package repository

import (
	"context"
	"testing"

	"product-service/internal/model"
	"product-service/internal/stock"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateStockDeductionLogIsUniqueByOrderID(t *testing.T) {
	repo := newTestProductRepository(t)
	ctx := context.Background()

	created, err := repo.CreateStockDeductionLog(ctx, &model.StockDeductionLog{
		OrderID:   "ORD-1",
		ProductID: 1,
		Quantity:  1,
		Status:    model.StockDeductionStatusProcessing,
		Reason:    "stock_deduction_started",
	})
	if err != nil {
		t.Fatalf("create stock deduction log failed: %v", err)
	}
	if !created {
		t.Fatal("expected first stock deduction log insert to be created")
	}

	created, err = repo.CreateStockDeductionLog(ctx, &model.StockDeductionLog{
		OrderID:   "ORD-1",
		ProductID: 1,
		Quantity:  1,
		Status:    model.StockDeductionStatusProcessing,
		Reason:    "duplicate_delivery",
	})
	if err != nil {
		t.Fatalf("duplicate stock deduction log insert failed: %v", err)
	}
	if created {
		t.Fatal("expected duplicate order_id insert to be ignored")
	}

	var count int64
	if err := repo.db.WithContext(ctx).Model(&model.StockDeductionLog{}).Where("order_id = ?", "ORD-1").Count(&count).Error; err != nil {
		t.Fatalf("count stock deduction logs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one stock deduction log, got %d", count)
	}
}

func TestUpdateStockDeductionLogStatus(t *testing.T) {
	repo := newTestProductRepository(t)
	ctx := context.Background()

	created, err := repo.CreateStockDeductionLog(ctx, &model.StockDeductionLog{
		OrderID:   "ORD-2",
		ProductID: 1,
		Quantity:  1,
		Status:    model.StockDeductionStatusProcessing,
		Reason:    "stock_deduction_started",
	})
	if err != nil {
		t.Fatalf("create stock deduction log failed: %v", err)
	}
	if !created {
		t.Fatal("expected stock deduction log to be created")
	}

	if err := repo.UpdateStockDeductionLog(ctx, "ORD-2", model.StockDeductionStatusSuccess, "stock deducted"); err != nil {
		t.Fatalf("update stock deduction log failed: %v", err)
	}

	var saved model.StockDeductionLog
	if err := repo.db.WithContext(ctx).Where("order_id = ?", "ORD-2").First(&saved).Error; err != nil {
		t.Fatalf("query stock deduction log failed: %v", err)
	}
	if saved.Status != model.StockDeductionStatusSuccess {
		t.Fatalf("expected status %s, got %s", model.StockDeductionStatusSuccess, saved.Status)
	}
	if saved.Reason != "stock deducted" {
		t.Fatalf("expected reason %q, got %q", "stock deducted", saved.Reason)
	}
}

func TestDeductStockAndMarkLogUpdatesProductAndLog(t *testing.T) {
	repo := newTestProductRepository(t)
	ctx := context.Background()

	product := &model.Product{Name: "phone", Price: 100, Stock: 2}
	if err := repo.Create(ctx, product); err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	created, err := repo.CreateStockDeductionLog(ctx, &model.StockDeductionLog{
		OrderID:   "ORD-3",
		ProductID: int32(product.ID),
		Quantity:  1,
		Status:    model.StockDeductionStatusProcessing,
		Reason:    stock.ResultReasonRedisDeducted,
	})
	if err != nil {
		t.Fatalf("create stock deduction log failed: %v", err)
	}
	if !created {
		t.Fatal("expected stock deduction log to be created")
	}

	updated, err := repo.DeductStockAndMarkLog(ctx, int32(product.ID), 1, "ORD-3", stock.ResultReasonMySQLDeducted)
	if err != nil {
		t.Fatalf("deduct stock and mark log failed: %v", err)
	}
	if !updated {
		t.Fatal("expected stock to be deducted")
	}

	savedProduct, err := repo.FindByID(ctx, int32(product.ID))
	if err != nil {
		t.Fatalf("find product failed: %v", err)
	}
	if savedProduct.Stock != 1 {
		t.Fatalf("expected stock 1, got %d", savedProduct.Stock)
	}

	var savedLog model.StockDeductionLog
	if err := repo.db.WithContext(ctx).Where("order_id = ?", "ORD-3").First(&savedLog).Error; err != nil {
		t.Fatalf("query stock deduction log failed: %v", err)
	}
	if savedLog.Status != model.StockDeductionStatusProcessing {
		t.Fatalf("expected status %s, got %s", model.StockDeductionStatusProcessing, savedLog.Status)
	}
	if savedLog.Reason != stock.ResultReasonMySQLDeducted {
		t.Fatalf("expected reason %q, got %q", stock.ResultReasonMySQLDeducted, savedLog.Reason)
	}
}

func newTestProductRepository(t *testing.T) *ProductRepository {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}

	repo := NewProductRepository(db)
	if err := repo.AutoMigrate(); err != nil {
		t.Fatalf("migrate product tables failed: %v", err)
	}
	return repo
}
