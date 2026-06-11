package mq

import (
	"context"
	"encoding/json"

	"product-service/internal/service"

	amqp "github.com/rabbitmq/amqp091-go"
)

// StockResultPublisher 库存扣减结果发布器
type StockResultPublisher struct {
	ch *amqp.Channel
}

// NewStockResultPublisher 创建结果发布器实例
func NewStockResultPublisher(ch *amqp.Channel) *StockResultPublisher {
	return &StockResultPublisher{ch: ch}
}

// PublishStockResult 将库存扣减结果发布到 stock_result_queue
func (p *StockResultPublisher) PublishStockResult(ctx context.Context, msg service.StockDeduction, success bool, reason string) error {
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

	return p.ch.PublishWithContext(ctx, "", StockResultQueue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
