package mq

import (
	"context"
	"encoding/json"
	"log"

	"order-service/internal/metrics"
	"order-service/internal/model"
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
			metrics.MQConsumeTotal.WithLabelValues(StockResultQueue).Inc()
			var result service.StockResult
			if err := json.Unmarshal(d.Body, &result); err != nil {
				metrics.MQConsumeFailedTotal.WithLabelValues(StockResultQueue).Inc()
				log.Printf("parse stock result failed: %v", err)
				_ = d.Nack(false, false)
				continue
			}

			if err := orderService.ApplyStockResult(context.Background(), result); err != nil {
				metrics.MQConsumeFailedTotal.WithLabelValues(StockResultQueue).Inc()
				log.Printf("apply stock result failed: order_id=%s err=%v", result.OrderID, err)
				_ = d.Nack(false, true)
				continue
			}

			log.Printf(
				"stock result handled: order_id=%s expected_status=%s target_status=%s reason=%s",
				result.OrderID,
				model.OrderStatusPendingStock,
				model.StatusFromStockResult(result.Success),
				"stock_result_consumed",
			)
			_ = d.Ack(false)
		}
	}()

	return nil
}
