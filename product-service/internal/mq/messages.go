package mq

import amqp "github.com/rabbitmq/amqp091-go"

// RabbitMQ 队列名称常量
const (
	StockQueue            = "stock_queue"
	StockRetryQueue       = "stock.deduct.retry.queue"
	StockDeadQueue        = "stock.deduct.dead.queue"
	StockResultQueue      = "stock_result_queue"
	MaxStockDeductRetries = 3
)

// StockDeductionMessage 库存扣减 RabbitMQ 消息体
type StockDeductionMessage struct {
	OrderID    string `json:"order_id"`
	UserID     int32  `json:"user_id"`
	ProductID  int32  `json:"product_id"`
	RetryCount int32  `json:"retry_count"`
	Error      string `json:"error,omitempty"`
}

// StockResultMessage 库存扣减结果消息体
type StockResultMessage struct {
	OrderID   string `json:"order_id"`
	UserID    int32  `json:"user_id"`
	ProductID int32  `json:"product_id"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason"`
}

// DeclareQueues 声明所有消息队列（持久化、非排他、非自动删除）
func DeclareQueues(ch *amqp.Channel) error {
	if _, err := ch.QueueDeclare(StockQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(StockRetryQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(StockDeadQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(StockResultQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return nil
}
