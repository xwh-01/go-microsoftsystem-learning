package mq

import (
	"context"
	"encoding/json"

	"product-service/internal/service"

	amqp "github.com/rabbitmq/amqp091-go"
)

type StockResultPublisher struct {
	ch *amqp.Channel
}

func NewStockResultPublisher(ch *amqp.Channel) *StockResultPublisher {
	return &StockResultPublisher{ch: ch}
}

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
