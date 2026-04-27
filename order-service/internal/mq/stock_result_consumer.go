package mq

import (
	"context"
	"encoding/json"
	"log"

	"order-service/internal/service"

	amqp "github.com/rabbitmq/amqp091-go"
)

func StartStockResultConsumer(ch *amqp.Channel, orderService *service.OrderService) error {
	msgs, err := ch.Consume(StockResultQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for d := range msgs {
			var result service.StockResult
			if err := json.Unmarshal(d.Body, &result); err != nil {
				log.Printf("parse stock result failed: %v", err)
				_ = d.Nack(false, false)
				continue
			}

			if err := orderService.ApplyStockResult(context.Background(), result); err != nil {
				log.Printf("apply stock result failed: order_id=%s err=%v", result.OrderID, err)
				_ = d.Nack(false, true)
				continue
			}

			log.Printf("order status updated: order_id=%s success=%v", result.OrderID, result.Success)
			_ = d.Ack(false)
		}
	}()

	return nil
}
