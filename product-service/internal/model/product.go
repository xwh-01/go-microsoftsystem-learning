package model

import "gorm.io/gorm"

// Product 商品表模型
type Product struct {
	gorm.Model
	Name    string  `gorm:"type:varchar(100);not null"`
	Price   float32 `gorm:"type:decimal(10,2)"`
	Stock   int32   `gorm:"not null;check:stock >= 0"`
	Version int32   `gorm:"default:1"`
}
