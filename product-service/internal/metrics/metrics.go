package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	StockDeductAttemptTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "stock_deduct_attempt_total",
		Help: "Total number of stock deduction processing attempts.",
	})

	StockDeductSuccessTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "stock_deduct_success_total",
		Help: "Total number of successful stock deductions.",
	})

	StockDeductFailedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stock_deduct_failed_total",
		Help: "Total number of failed stock deductions by reason.",
	}, []string{"reason"})

	MQConsumeTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mq_consume_total",
		Help: "Total number of RabbitMQ messages consumed.",
	}, []string{"queue"})

	MQConsumeFailedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mq_consume_failed_total",
		Help: "Total number of RabbitMQ messages that failed processing.",
	}, []string{"queue"})

	MQRetryTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mq_retry_total",
		Help: "Total number of RabbitMQ messages published to retry queues.",
	}, []string{"queue"})

	MQDeadTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mq_dead_total",
		Help: "Total number of RabbitMQ messages published to dead queues.",
	}, []string{"queue"})
)

func init() {
	prometheus.MustRegister(
		StockDeductAttemptTotal,
		StockDeductSuccessTotal,
		StockDeductFailedTotal,
		MQConsumeTotal,
		MQConsumeFailedTotal,
		MQRetryTotal,
		MQDeadTotal,
	)
}
