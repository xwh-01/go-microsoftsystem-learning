package repository

import (
	"context"
	"errors"

	"product-service/internal/model"

	"gorm.io/gorm"
)

var ErrProductNotFound = errors.New("product not found")

type ProductRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

func (r *ProductRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&model.Product{})
}

func (r *ProductRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Product{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *ProductRepository) Create(ctx context.Context, product *model.Product) error {
	return r.db.WithContext(ctx).Create(product).Error
}

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
