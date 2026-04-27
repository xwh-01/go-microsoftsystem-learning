package repository

import (
	"context"
	"errors"

	"order-service/internal/model"

	"gorm.io/gorm"
)

var ErrOrderNotFound = errors.New("order not found")

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&model.Order{})
}

func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	return r.db.WithContext(ctx).Create(order).Error
}

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

func (r *OrderRepository) UpdateStatus(ctx context.Context, orderID string, status string, message string) error {
	return r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("order_id = ?", orderID).
		Updates(map[string]any{
			"status":  status,
			"message": message,
		}).Error
}
