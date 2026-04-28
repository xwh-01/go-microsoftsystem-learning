package mq

import (
	"context"
	"encoding/json"
	"log"

	"product-service/internal/service"

	amqp "github.com/rabbitmq/amqp091-go"
)

func StartStockConsumer(ch *amqp.Channel, processor *service.StockProcessor) error {
	msgs, err := ch.Consume(StockQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	log.Println("stock consumer started")

	go func() {
		for d := range msgs {
			var msg StockDeductionMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				log.Printf("parse stock deduction message failed: %v", err)
				_ = d.Nack(false, false)
				continue
			}

			ack, err := processor.Process(context.Background(), service.StockDeduction{
				OrderID:   msg.OrderID,
				UserID:    msg.UserID,
				ProductID: msg.ProductID,
			})
			if err != nil {
				_ = d.Nack(false, true)
				continue
			}
			if ack {
				_ = d.Ack(false)
				continue
			}
			_ = d.Nack(false, true)
		}
	}()

	return nil
}
