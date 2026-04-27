package model

import "gorm.io/gorm"

const (
	OrderStatusPendingStock = "pending_stock"
	OrderStatusConfirmed    = "confirmed"
	OrderStatusFailed       = "failed"
)

const (
	OrderMessageWaitingStock = "waiting for stock deduction"
	OrderMessageConfirmed    = "order confirmed"
	OrderMessageFailed       = "stock deduction failed"
)

type Order struct {
	gorm.Model
	OrderID   string `gorm:"column:order_id;type:varchar(64);uniqueIndex;not null"`
	UserID    int32  `gorm:"column:user_id;not null"`
	ProductID int32  `gorm:"column:product_id;not null"`
	Status    string `gorm:"type:varchar(32);not null;index"`
	Message   string `gorm:"type:varchar(255)"`
}

func StatusFromStockResult(success bool) string {
	if success {
		return OrderStatusConfirmed
	}
	return OrderStatusFailed
}

func MessageFromStockResult(success bool, reason string) string {
	if success {
		return OrderMessageConfirmed
	}
	if reason != "" {
		return reason
	}
	return OrderMessageFailed
}
