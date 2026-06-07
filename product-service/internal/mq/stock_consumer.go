package mq

import (
	"context"
	"encoding/json"
	"log"

	"product-service/internal/metrics"
	"product-service/internal/service"

	amqp "github.com/rabbitmq/amqp091-go"
)

func StartStockConsumer(ch *amqp.Channel, processor *service.StockProcessor) error {
	if err := startStockConsumerForQueue(ch, processor, StockQueue); err != nil {
		return err
	}
	if err := startStockConsumerForQueue(ch, processor, StockRetryQueue); err != nil {
		return err
	}
	log.Printf("stock consumer started: queues=%s,%s", StockQueue, StockRetryQueue)

	return nil
}

func startStockConsumerForQueue(ch *amqp.Channel, processor *service.StockProcessor, queue string) error {
	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for d := range msgs {
			metrics.MQConsumeTotal.WithLabelValues(queue).Inc()
			var msg StockDeductionMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				metrics.MQConsumeFailedTotal.WithLabelValues(queue).Inc()
				log.Printf("parse stock deduction message failed: %v", err)
				_ = d.Nack(false, false)
				continue
			}

			ack, err := processor.Process(context.Background(), service.StockDeduction{
				OrderID:    msg.OrderID,
				UserID:     msg.UserID,
				ProductID:  msg.ProductID,
				RetryCount: msg.RetryCount,
			})
			if err != nil {
				metrics.MQConsumeFailedTotal.WithLabelValues(queue).Inc()
				if publishErr := publishRetryOrDead(context.Background(), ch, processor, msg, err); publishErr != nil {
					log.Printf(
						"stock deduction retry routing failed: order_id=%s product_id=%d retry_count=%d error=%v next_action=%s publish_error=%v",
						msg.OrderID,
						msg.ProductID,
						msg.RetryCount,
						err,
						"retry",
						publishErr,
					)
					_ = d.Nack(false, true)
					continue
				}
				_ = d.Ack(false)
				continue
			}
			if ack {
				_ = d.Ack(false)
				continue
			}
			_ = d.Nack(false, true)
			metrics.MQConsumeFailedTotal.WithLabelValues(queue).Inc()
		}
	}()

	return nil
}

func publishRetryOrDead(ctx context.Context, ch *amqp.Channel, processor *service.StockProcessor, msg StockDeductionMessage, err error) error {
	msg.Error = err.Error()
	if msg.RetryCount < MaxStockDeductRetries {
		msg.RetryCount++
		if publishErr := publishStockDeductionMessage(ctx, ch, StockRetryQueue, msg); publishErr != nil {
			return publishErr
		}
		metrics.MQRetryTotal.WithLabelValues(StockRetryQueue).Inc()
		log.Printf(
			"stock deduction failed: order_id=%s product_id=%d retry_count=%d error=%v next_action=%s",
			msg.OrderID,
			msg.ProductID,
			msg.RetryCount,
			err,
			"retry",
		)
		return nil
	}

	if markErr := processor.MarkRetryExhausted(ctx, service.StockDeduction{
		OrderID:    msg.OrderID,
		UserID:     msg.UserID,
		ProductID:  msg.ProductID,
		RetryCount: msg.RetryCount,
	}, err.Error()); markErr != nil {
		log.Printf(
			"mark stock retry exhausted failed: order_id=%s product_id=%d retry_count=%d error=%v next_action=%s mark_error=%v",
			msg.OrderID,
			msg.ProductID,
			msg.RetryCount,
			err,
			"dead",
			markErr,
		)
	}
	if publishErr := publishStockDeductionMessage(ctx, ch, StockDeadQueue, msg); publishErr != nil {
		return publishErr
	}
	metrics.MQDeadTotal.WithLabelValues(StockDeadQueue).Inc()
	log.Printf(
		"stock deduction failed: order_id=%s product_id=%d retry_count=%d error=%v next_action=%s",
		msg.OrderID,
		msg.ProductID,
		msg.RetryCount,
		err,
		"dead",
	)
	return nil
}

func publishStockDeductionMessage(ctx context.Context, ch *amqp.Channel, queue string, msg StockDeductionMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
