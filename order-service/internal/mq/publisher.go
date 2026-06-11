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

// Publisher 订单服务消息发布器
type Publisher struct {
	ch *amqp.Channel
}

// NewPublisher 创建消息发布器
func NewPublisher(ch *amqp.Channel) *Publisher {
	return &Publisher{ch: ch}
}

// DeclareQueues 声明订单服务所需的 RabbitMQ 队列
func DeclareQueues(ch *amqp.Channel) error {
	if _, err := ch.QueueDeclare(StockQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(StockResultQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return nil
}

// PublishStockDeduction 发布库存扣减请求到 stock_queue
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
