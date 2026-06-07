package model

import "time"

const (
	StockDeductionStatusProcessing = "processing"
	StockDeductionStatusSuccess    = "success"
	StockDeductionStatusFailed     = "failed"
	StockDeductionStatusSkipped    = "skipped"
)

type StockDeductionLog struct {
	ID        uint      `gorm:"primaryKey"`
	OrderID   string    `gorm:"column:order_id;type:varchar(64);uniqueIndex;not null"`
	ProductID int32     `gorm:"column:product_id;not null"`
	Quantity  int32     `gorm:"column:quantity;not null"`
	Status    string    `gorm:"column:status;type:varchar(32);not null;index"`
	Reason    string    `gorm:"column:reason;type:varchar(255)"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}
