package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	pb "micro-proto"
	"order-service/internal/mq"
	"order-service/internal/repository"
	"order-service/internal/service"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// grpcOrderHandler 实现 proto 定义的 OrderServiceServer 接口
type grpcOrderHandler struct {
	pb.UnimplementedOrderServiceServer
	orderService *service.OrderService
}

// CreateOrder 处理下单请求
func (h *grpcOrderHandler) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	log.Printf("create order request: user_id=%d product_id=%d", req.UserId, req.ProductId)

	order, err := h.orderService.CreateOrder(ctx, req.UserId, req.ProductId)
	if err != nil {
		log.Printf("create order failed: %v", err)
		return &pb.CreateOrderResponse{Code: 500, Message: "create order failed"}, nil
	}

	return &pb.CreateOrderResponse{
		Code:    200,
		Message: "order accepted",
		OrderId: order.OrderID,
	}, nil
}

// GetOrder 查询订单详情
func (h *grpcOrderHandler) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderResponse, error) {
	order, err := h.orderService.GetOrder(ctx, req.OrderId)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			return &pb.GetOrderResponse{Code: 404, Message: "order not found"}, nil
		}
		log.Printf("query order failed: order_id=%s err=%v", req.OrderId, err)
		return &pb.GetOrderResponse{Code: 500, Message: "query order failed"}, nil
	}

	return &pb.GetOrderResponse{
		Code:          200,
		Message:       "ok",
		OrderId:       order.OrderID,
		UserId:        order.UserID,
		ProductId:     order.ProductID,
		Status:        order.Status,
		StatusMessage: order.Message,
		CreatedAt:     order.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     order.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// order-service 订单服务主入口
// 职责：创建订单、发布库存扣减消息、消费扣减结果并更新订单状态
func main() {
	viper.SetConfigFile("../config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("read config failed: %v", err)
	}
	startMetricsServer("order-service", viper.GetString("order_service.metrics_port"), ":9103")

	db, err := gorm.Open(mysql.Open(viper.GetString("mysql_dsn")), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect MySQL failed: %v", err)
	}
	orderRepo := repository.NewOrderRepository(db)
	if err := orderRepo.AutoMigrate(); err != nil {
		log.Fatalf("migrate order table failed: %v", err)
	}

	mqConn, err := amqp.Dial(viper.GetString("common.rabbitmq_addr"))
	if err != nil {
		log.Fatalf("connect RabbitMQ failed: %v", err)
	}
	defer mqConn.Close()

	mqChannel, err := mqConn.Channel()
	if err != nil {
		log.Fatalf("open RabbitMQ channel failed: %v", err)
	}
	defer mqChannel.Close()

	if err := mq.DeclareQueues(mqChannel); err != nil {
		log.Fatalf("declare RabbitMQ queues failed: %v", err)
	}

	orderService := service.NewOrderService(orderRepo, mq.NewPublisher(mqChannel))
	if err := mq.StartStockResultConsumer(mqChannel, orderService); err != nil {
		log.Fatalf("start stock result consumer failed: %v", err)
	}

	port := viper.GetString("order_service.port")
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterOrderServiceServer(grpcServer, &grpcOrderHandler{orderService: orderService})

	log.Printf("order service started on %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve failed: %v", err)
	}
}

// startMetricsServer 启动 Prometheus metrics HTTP 服务器
func startMetricsServer(serviceName string, configuredAddr string, defaultAddr string) {
	addr := configuredAddr
	if addr == "" {
		addr = defaultAddr
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("%s metrics started on %s", serviceName, addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("%s metrics server failed: %v", serviceName, err)
		}
	}()
}
