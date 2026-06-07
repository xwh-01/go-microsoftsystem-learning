package main

import (
	"context"
	"log"
	"net"
	"net/http"

	pb "micro-proto"
	productgrpc "product-service/internal/grpc"
	"product-service/internal/model"
	productmq "product-service/internal/mq"
	"product-service/internal/repository"
	"product-service/internal/service"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	viper.SetConfigFile("../config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("read config failed: %v", err)
	}
	startMetricsServer("product-service", viper.GetString("product_service.metrics_port"), ":9102")

	ctx := context.Background()
	productRepo := initProductRepository(ctx)
	rdb := initRedis()
	mqConn, mqChannel := initRabbitMQ()
	defer mqConn.Close()
	defer mqChannel.Close()

	stockProcessor := service.NewStockProcessor(
		productRepo,
		rdb,
		productmq.NewStockResultPublisher(mqChannel),
	)
	if err := productmq.StartStockConsumer(mqChannel, stockProcessor); err != nil {
		log.Fatalf("start stock consumer failed: %v", err)
	}

	port := viper.GetString("product_service.port")
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	grpcServer := grpc.NewServer()
	productService := service.NewProductService(productRepo, rdb)
	pb.RegisterProductServiceServer(grpcServer, productgrpc.NewHandler(productService))

	log.Printf("product service started on %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve failed: %v", err)
	}
}

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

func initProductRepository(ctx context.Context) *repository.ProductRepository {
	db, err := gorm.Open(mysql.Open(viper.GetString("mysql_dsn")), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect MySQL failed: %v", err)
	}

	productRepo := repository.NewProductRepository(db)
	if err := productRepo.AutoMigrate(); err != nil {
		log.Fatalf("migrate product table failed: %v", err)
	}
	seedProduct(ctx, productRepo)
	return productRepo
}

func seedProduct(ctx context.Context, repo *repository.ProductRepository) {
	count, err := repo.Count(ctx)
	if err != nil {
		log.Fatalf("count products failed: %v", err)
	}
	if count > 0 {
		return
	}

	product := &model.Product{Name: "iPhone 16 Pro", Price: 8999.00, Stock: 100, Version: 1}
	if err := repo.Create(ctx, product); err != nil {
		log.Fatalf("seed product failed: %v", err)
	}
	log.Println("seed product created")
}

func initRedis() *redis.Client {
	rdb := redis.NewClient(&redis.Options{Addr: viper.GetString("common.redis_addr")})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("connect Redis failed: %v", err)
	}
	return rdb
}

func initRabbitMQ() (*amqp.Connection, *amqp.Channel) {
	conn, err := amqp.Dial(viper.GetString("common.rabbitmq_addr"))
	if err != nil {
		log.Fatalf("connect RabbitMQ failed: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		log.Fatalf("open RabbitMQ channel failed: %v", err)
	}

	if err := productmq.DeclareQueues(ch); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		log.Fatalf("declare stock queues failed: %v", err)
	}
	return conn, ch
}
