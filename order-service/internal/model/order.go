package model

import "gorm.io/gorm"

// 订单状态常量
const (
	OrderStatusPendingStock = "pending_stock" // 等待库存扣减
	OrderStatusConfirmed    = "confirmed"     // 订单确认
	OrderStatusFailed       = "failed"        // 订单失败
)

// 订单消息常量
const (
	OrderMessageWaitingStock = "waiting for stock deduction"
	OrderMessageConfirmed    = "order confirmed"
	OrderMessageFailed       = "stock deduction failed"
)

// Order 订单表模型
type Order struct {
	gorm.Model
	OrderID   string `gorm:"column:order_id;type:varchar(64);uniqueIndex;not null"`
	UserID    int32  `gorm:"column:user_id;not null"`
	ProductID int32  `gorm:"column:product_id;not null"`
	Status    string `gorm:"type:varchar(32);not null;index"`
	Message   string `gorm:"type:varchar(255)"`
}

// StatusFromStockResult 根据库存扣减结果映射订单状态
func StatusFromStockResult(success bool) string {
	if success {
		return OrderStatusConfirmed
	}
	return OrderStatusFailed
}

// MessageFromStockResult 根据库存扣减结果生成订单状态消息
func MessageFromStockResult(success bool, reason string) string {
	if success {
		return OrderMessageConfirmed
	}
	if reason != "" {
		return reason
	}
	return OrderMessageFailed
}
