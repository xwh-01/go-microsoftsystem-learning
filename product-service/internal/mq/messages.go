package mq

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	StockQueue       = "stock_queue"
	StockResultQueue = "stock_result_queue"
)

type StockDeductionMessage struct {
	OrderID   string `json:"order_id"`
	UserID    int32  `json:"user_id"`
	ProductID int32  `json:"product_id"`
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
	if _, err := ch.QueueDeclare(StockResultQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return nil
}

func PublishStockResult(ctx context.Context, ch *amqp.Channel, msg StockDeductionMessage, success bool, reason string) error {
	result := StockResultMessage{
		OrderID:   msg.OrderID,
		UserID:    msg.UserID,
		ProductID: msg.ProductID,
		Success:   success,
		Reason:    reason,
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return ch.PublishWithContext(ctx, "", StockResultQueue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
