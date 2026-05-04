package main

import (
	"context"
	"errors"
	"log"
	"net"

	pb "micro-proto"
	"order-service/internal/mq"
	"order-service/internal/repository"
	"order-service/internal/service"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type grpcOrderHandler struct {
	pb.UnimplementedOrderServiceServer
	orderService *service.OrderService
}

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
	}, nil
}

func main() {
	viper.SetConfigFile("../config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("read config failed: %v", err)
	}

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
