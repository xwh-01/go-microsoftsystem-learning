package metrics

import "github.com/prometheus/client_golang/prometheus"

// Prometheus 指标定义
var (
	// OrdersCreatedTotal 创建的订单总数
	OrdersCreatedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "orders_created_total",
		Help: "Total number of orders created in pending_stock status.",
	})

	OrdersConfirmedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "orders_confirmed_total",
		Help: "Total number of orders confirmed after stock deduction.",
	})

	OrdersFailedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "orders_failed_total",
		Help: "Total number of orders marked failed.",
	})

	OrderStatusUpdateTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "order_status_update_total",
		Help: "Total number of order status update attempts.",
	}, []string{"from_status", "to_status", "result"})

	MQConsumeTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mq_consume_total",
		Help: "Total number of RabbitMQ messages consumed.",
	}, []string{"queue"})

	MQConsumeFailedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mq_consume_failed_total",
		Help: "Total number of RabbitMQ messages that failed processing.",
	}, []string{"queue"})
)

func init() {
	prometheus.MustRegister(
		OrdersCreatedTotal,
		OrdersConfirmedTotal,
		OrdersFailedTotal,
		OrderStatusUpdateTotal,
		MQConsumeTotal,
		MQConsumeFailedTotal,
	)
}
