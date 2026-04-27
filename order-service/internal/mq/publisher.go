package mq

import (
	"context"
	"encoding/json"

	"order-service/internal/service"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	StockQueue       = "stock_queue"
	StockResultQueue = "stock_result_queue"
)

type Publisher struct {
	ch *amqp.Channel
}

func NewPublisher(ch *amqp.Channel) *Publisher {
	return &Publisher{ch: ch}
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

func (p *Publisher) PublishStockDeduction(ctx context.Context, msg service.StockDeduction) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return p.ch.PublishWithContext(ctx, "", StockQueue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
