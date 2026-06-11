package repository

import (
	"context"
	"errors"

	"order-service/internal/model"

	"gorm.io/gorm"
)

var ErrOrderNotFound = errors.New("order not found")

// OrderRepository 订单数据访问层
type OrderRepository struct {
	db *gorm.DB
}

// NewOrderRepository 创建订单仓储实例
func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// AutoMigrate 自动迁移 Order 表结构
func (r *OrderRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&model.Order{})
}

// Create 创建新订单记录
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	return r.db.WithContext(ctx).Create(order).Error
}

// FindByOrderID 按 order_id 查询订单
func (r *OrderRepository) FindByOrderID(ctx context.Context, orderID string) (*model.Order, error) {
	var order model.Order
	if err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// UpdateStatus 直接更新订单状态
func (r *OrderRepository) UpdateStatus(ctx context.Context, orderID string, status string, message string) error {
	return r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("order_id = ?", orderID).
		Updates(map[string]any{
			"status":  status,
			"message": message,
		}).Error
}

// UpdateStatusFrom 条件更新：仅当订单处于 expectedStatus 时才更新为 targetStatus
// 防止重复消息或乱序消息覆盖最终状态（confirmed/failed）
func (r *OrderRepository) UpdateStatusFrom(ctx context.Context, orderID string, expectedStatus string, targetStatus string, message string) (bool, error) {
	tx := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("order_id = ? AND status = ?", orderID, expectedStatus).
		Updates(map[string]any{
			"status":  targetStatus,
			"message": message,
		})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}
