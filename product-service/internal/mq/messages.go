package mq

import amqp "github.com/rabbitmq/amqp091-go"

const (
	StockQueue            = "stock_queue"
	StockRetryQueue       = "stock.deduct.retry.queue"
	StockDeadQueue        = "stock.deduct.dead.queue"
	StockResultQueue      = "stock_result_queue"
	MaxStockDeductRetries = 3
)

type StockDeductionMessage struct {
	OrderID    string `json:"order_id"`
	UserID     int32  `json:"user_id"`
	ProductID  int32  `json:"product_id"`
	RetryCount int32  `json:"retry_count"`
	Error      string `json:"error,omitempty"`
}

type StockResultMessage struct {
	OrderID   string `json:"order_id"`
	UserID    int32  `json:"user_id"`
	ProductID int32  `json:"product_id"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason"`
}

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
