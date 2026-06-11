package repository

import (
	"context"
	"errors"

	"product-service/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrProductNotFound           = errors.New("product not found")
	ErrStockDeductionLogNotFound = errors.New("stock deduction log not found")
)

// ProductRepository 商品数据访问层，封装 GORM 数据库操作
type ProductRepository struct {
	db *gorm.DB
}

// NewProductRepository 创建商品仓储实例
func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// AutoMigrate 自动迁移 Product 和 StockDeductionLog 表结构
func (r *ProductRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&model.Product{}, &model.StockDeductionLog{})
}

// Count 统计商品总数（用于判断是否需要种子数据）
func (r *ProductRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Product{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Create 插入新商品记录
func (r *ProductRepository) Create(ctx context.Context, product *model.Product) error {
	return r.db.WithContext(ctx).Create(product).Error
}

// FindByID 按主键 ID 查询商品
func (r *ProductRepository) FindByID(ctx context.Context, productID int32) (*model.Product, error) {
	var product model.Product
	if err := r.db.WithContext(ctx).Where("id = ?", productID).First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	return &product, nil
}

// DeductStock 条件扣减库存：UPDATE stock = stock - ? WHERE id = ? AND stock >= ?
func (r *ProductRepository) DeductStock(ctx context.Context, productID int32, quantity int32) (bool, error) {
	tx := r.db.WithContext(ctx).
		Model(&model.Product{}).
		Where("id = ? AND stock >= ?", productID, quantity).
		Update("stock", gorm.Expr("stock - ?", quantity))
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

// DeductStockAndMarkLog 事务内：条件扣库存 + 更新扣减日志状态
func (r *ProductRepository) DeductStockAndMarkLog(ctx context.Context, productID int32, quantity int32, orderID string, reason string) (bool, error) {
	var updated bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.
			Model(&model.Product{}).
			Where("id = ? AND stock >= ?", productID, quantity).
			Update("stock", gorm.Expr("stock - ?", quantity))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		if err := tx.
			Model(&model.StockDeductionLog{}).
			Where("order_id = ?", orderID).
			Updates(map[string]any{
				"status": model.StockDeductionStatusProcessing,
				"reason": reason,
			}).Error; err != nil {
			return err
		}
		updated = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return updated, nil
}

// CreateStockDeductionLog 创建扣减日志，order_id 唯一冲突时忽略（ON CONFLICT DO NOTHING）
func (r *ProductRepository) CreateStockDeductionLog(ctx context.Context, log *model.StockDeductionLog) (bool, error) {
	tx := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "order_id"}},
			DoNothing: true,
		}).
		Create(log)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

// FindStockDeductionLog 按 order_id 查询扣减日志
func (r *ProductRepository) FindStockDeductionLog(ctx context.Context, orderID string) (*model.StockDeductionLog, error) {
	var log model.StockDeductionLog
	if err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&log).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStockDeductionLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// UpdateStockDeductionLog 更新扣减日志的状态和原因
func (r *ProductRepository) UpdateStockDeductionLog(ctx context.Context, orderID string, status string, reason string) error {
	return r.db.WithContext(ctx).
		Model(&model.StockDeductionLog{}).
		Where("order_id = ?", orderID).
		Updates(map[string]any{
			"status": status,
			"reason": reason,
		}).Error
}
