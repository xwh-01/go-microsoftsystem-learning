package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "micro-proto"
	"product-service/internal/model"
	productmq "product-service/internal/mq"
	"product-service/internal/stock"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/hashicorp/consul/api"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type productService struct {
	pb.UnimplementedProductServiceServer
	rdb   *redis.Client
	db    *gorm.DB
	bloom *stock.BloomFilter
}

func (s *productService) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.GetProductResponse, error) {
	if !s.bloom.MightContainInt32(req.ProductId) {
		return nil, status.Errorf(codes.NotFound, "product id %d not found", req.ProductId)
	}

	cacheKey := stock.ProductCacheKey(req.ProductId)

	val, err := s.rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		if val == stock.NullProductCacheValue {
			return nil, status.Errorf(codes.NotFound, "product id %d not found", req.ProductId)
		}

		var p pb.GetProductResponse
		if err := json.Unmarshal([]byte(val), &p); err == nil {
			return &p, nil
		}
		_ = s.rdb.Del(ctx, cacheKey).Err()
	} else if err != redis.Nil {
		log.Printf("read product cache failed: %v", err)
	}

	var product model.Product
	if err := s.db.Where("id = ?", req.ProductId).First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = s.rdb.Set(ctx, cacheKey, stock.NullProductCacheValue, stock.NullProductCacheTTL).Err()
		}
		return nil, status.Errorf(codes.NotFound, "product id %d not found", req.ProductId)
	}

	p := &pb.GetProductResponse{
		Id:    req.ProductId,
		Name:  product.Name,
		Price: product.Price,
		Stock: product.Stock,
	}

	if jsonBytes, err := json.Marshal(p); err == nil {
		_ = s.rdb.Set(ctx, cacheKey, jsonBytes, stock.ProductCacheTTL).Err()
	}
	_ = s.rdb.Set(ctx, stock.StockCacheKey(req.ProductId), product.Stock, stock.StockCacheTTL).Err()
	s.bloom.AddInt32(req.ProductId)

	return p, nil
}

func startStockConsumer(rdb *redis.Client, db *gorm.DB) {
	pool := goredis.NewPool(rdb)
	rs := redsync.New(pool)

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

	msgs, err := ch.Consume(productmq.StockQueue, "", false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		log.Fatalf("start stock consumer failed: %v", err)
	}
	log.Println("stock consumer started")

	go func() {
		defer conn.Close()
		defer ch.Close()

		for d := range msgs {
			var msg productmq.StockDeductionMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				log.Printf("parse stock deduction message failed: %v", err)
				_ = d.Nack(false, false)
				continue
			}

			idempotentKey := stock.IdempotencyKey(msg.OrderID)
			processed, err := rdb.Exists(context.Background(), idempotentKey).Result()
			if err != nil {
				log.Printf("idempotency check failed: %v", err)
				_ = d.Nack(false, true)
				continue
			}
			if processed > 0 {
				log.Printf("duplicate order ignored: %s", msg.OrderID)
				_ = d.Ack(false)
				continue
			}

			cacheKey := stock.StockCacheKey(msg.ProductID)
			res, err := rdb.Eval(context.Background(), stock.LuaDeductStock, []string{cacheKey}, 1).Int()
			if err != nil {
				log.Printf("decrement stock in redis failed: %v", err)
				_ = d.Nack(false, true)
				continue
			}

			switch res {
			case stock.DeductSuccess:
				handleRedisStockDeducted(rdb, db, rs, ch, d, msg, idempotentKey)
			case stock.DeductInsufficient:
				handleInsufficientStock(rdb, ch, d, msg, idempotentKey)
			case stock.DeductCacheMissing:
				reloadStockCache(db, rdb, msg.ProductID, cacheKey)
				_ = d.Nack(false, true)
			default:
				log.Printf("unexpected stock deduction result: %d", res)
				_ = d.Nack(false, true)
			}
		}
	}()
}

func handleRedisStockDeducted(
	rdb *redis.Client,
	db *gorm.DB,
	rs *redsync.Redsync,
	ch *amqp.Channel,
	d amqp.Delivery,
	msg productmq.StockDeductionMessage,
	idempotentKey string,
) {
	mutex := rs.NewMutex(stock.ProductLockKey(msg.ProductID))
	if err := mutex.Lock(); err != nil {
		log.Printf("acquire stock lock failed: %v", err)
		_ = d.Nack(false, true)
		return
	}
	defer func() {
		_, _ = mutex.Unlock()
	}()

	tx := db.Model(&model.Product{}).
		Where("id = ? AND stock > 0", msg.ProductID).
		Update("stock", gorm.Expr("stock - 1"))

	if tx.Error != nil {
		log.Printf("update stock in mysql failed: %v", tx.Error)
		_ = d.Nack(false, true)
		return
	}

	if tx.RowsAffected == 0 {
		log.Printf("stock update skipped for order %s", msg.OrderID)
		if err := productmq.PublishStockResult(context.Background(), ch, msg, false, stock.ResultReasonDBUpdateSkipped); err != nil {
			log.Printf("publish stock result failed: %v", err)
			_ = d.Nack(false, true)
			return
		}
		_ = rdb.Set(context.Background(), idempotentKey, "1", 24*time.Hour).Err()
		_ = d.Ack(false)
		return
	}

	_ = rdb.Del(context.Background(), stock.ProductCacheKey(msg.ProductID)).Err()
	if err := productmq.PublishStockResult(context.Background(), ch, msg, true, stock.ResultReasonDeducted); err != nil {
		log.Printf("publish stock result failed: %v", err)
		_ = d.Nack(false, true)
		return
	}

	_ = rdb.Set(context.Background(), idempotentKey, "1", 24*time.Hour).Err()
	log.Printf("stock decremented for order %s", msg.OrderID)
	_ = d.Ack(false)
}

func handleInsufficientStock(
	rdb *redis.Client,
	ch *amqp.Channel,
	d amqp.Delivery,
	msg productmq.StockDeductionMessage,
	idempotentKey string,
) {
	log.Printf("insufficient stock: order_id=%s product_id=%d", msg.OrderID, msg.ProductID)
	if err := productmq.PublishStockResult(context.Background(), ch, msg, false, stock.ResultReasonInsufficient); err != nil {
		log.Printf("publish stock result failed: %v", err)
		_ = d.Nack(false, true)
		return
	}
	_ = rdb.Set(context.Background(), idempotentKey, "1", 24*time.Hour).Err()
	_ = d.Ack(false)
}

func reloadStockCache(db *gorm.DB, rdb *redis.Client, productID int32, cacheKey string) {
	var product model.Product
	if err := db.Where("id = ?", productID).First(&product).Error; err == nil {
		_ = rdb.Set(context.Background(), cacheKey, product.Stock, stock.StockCacheTTL).Err()
	}
}

func loadProductBloomFilter(db *gorm.DB) (*stock.BloomFilter, error) {
	var ids []int32
	if err := db.Model(&model.Product{}).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}

	bitSize := uint64(2048)
	if len(ids) > 0 {
		bitSize = uint64(len(ids) * 32)
	}
	filter := stock.NewBloomFilter(bitSize, 3)
	for _, id := range ids {
		filter.AddInt32(id)
	}
	return filter, nil
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
	if err := db.AutoMigrate(&model.Product{}); err != nil {
		log.Fatalf("migrate product table failed: %v", err)
	}

	var count int64
	db.Model(&model.Product{}).Count(&count)
	if count == 0 {
		if err := db.Create(&model.Product{Name: "iPhone 16 Pro", Price: 8999.00, Stock: 100, Version: 1}).Error; err != nil {
			log.Fatalf("seed product failed: %v", err)
		}
		log.Println("seed product created")
	}

	rdb := redis.NewClient(&redis.Options{Addr: viper.GetString("common.redis_addr")})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("connect Redis failed: %v", err)
	}

	bloomFilter, err := loadProductBloomFilter(db)
	if err != nil {
		log.Fatalf("load product bloom filter failed: %v", err)
	}

	startStockConsumer(rdb, db)

	port := viper.GetString("product_service.port")
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterProductServiceServer(grpcServer, &productService{rdb: rdb, db: db, bloom: bloomFilter})

	consulClient, err := api.NewClient(&api.Config{Address: viper.GetString("common.consul_addr")})
	if err != nil {
		log.Fatalf("create consul client failed: %v", err)
	}
	registration := &api.AgentServiceRegistration{
		ID:      viper.GetString("product_service.node_id"),
		Name:    viper.GetString("product_service.name"),
		Port:    50052,
		Address: "127.0.0.1",
	}
	if err := consulClient.Agent().ServiceRegister(registration); err != nil {
		log.Fatalf("register consul service failed: %v", err)
	}

	log.Printf("product service started on %s", port)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("serve failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	if err := consulClient.Agent().ServiceDeregister(viper.GetString("product_service.node_id")); err != nil {
		log.Printf("deregister consul service failed: %v", err)
	}
	grpcServer.GracefulStop()
	log.Println("product service stopped")
}
