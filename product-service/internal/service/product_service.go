package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	pb "micro-proto"
	"product-service/internal/model"
	"product-service/internal/repository"
	"product-service/internal/stock"

	"github.com/redis/go-redis/v9"
)

// productRepository 商品仓储接口，解耦具体数据库实现
type productRepository interface {
	Create(ctx context.Context, product *model.Product) error
	FindByID(ctx context.Context, productID int32) (*model.Product, error)
	DeductStock(ctx context.Context, productID int32, quantity int32) (bool, error)
	FindStockDeductionLog(ctx context.Context, orderID string) (*model.StockDeductionLog, error)
}

// ProductService 商品查询服务，集成 Redis 缓存层
type ProductService struct {
	repo productRepository
	rdb  *redis.Client
}

// NewProductService 创建商品服务实例
func NewProductService(repo productRepository, rdb *redis.Client) *ProductService {
	return &ProductService{
		repo: repo,
		rdb:  rdb,
	}
}

// GetProduct 获取商品信息，优先读 Redis 缓存（支持空对象缓存防穿透）
func (s *ProductService) GetProduct(ctx context.Context, productID int32) (*pb.GetProductResponse, error) {
	cacheKey := stock.ProductCacheKey(productID)
	cached, ok, notFound := s.readProductCache(ctx, cacheKey)
	if notFound {
		return nil, ErrProductNotFound
	}
	if ok {
		return cached, nil
	}

	product, err := s.repo.FindByID(ctx, productID)
	if err != nil {
		if errors.Is(err, repository.ErrProductNotFound) {
			_ = s.rdb.Set(ctx, cacheKey, stock.NullProductCacheValue, stock.NullProductCacheTTL).Err()
			return nil, ErrProductNotFound
		}
		return nil, fmt.Errorf("query product failed: %w", err)
	}

	res := &pb.GetProductResponse{
		Id:    productID,
		Name:  product.Name,
		Price: product.Price,
		Stock: product.Stock,
	}
	s.writeProductCache(ctx, cacheKey, res)
	_ = s.rdb.Set(ctx, stock.StockCacheKey(productID), product.Stock, stock.StockCacheTTL).Err()

	return res, nil
}

// GetStockDeductionLog 查询指定订单的库存扣减日志
func (s *ProductService) GetStockDeductionLog(ctx context.Context, orderID string) (*model.StockDeductionLog, error) {
	logEntry, err := s.repo.FindStockDeductionLog(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrStockDeductionLogNotFound) {
			return nil, ErrStockDeductionLogNotFound
		}
		return nil, fmt.Errorf("query stock deduction log failed: %w", err)
	}
	return logEntry, nil
}

// readProductCache 从 Redis 读取商品缓存，返回(商品数据, 是否命中, 是否空对象标记)
func (s *ProductService) readProductCache(ctx context.Context, cacheKey string) (*pb.GetProductResponse, bool, bool) {
	val, err := s.rdb.Get(ctx, cacheKey).Result()
	if err == redis.Nil {
		return nil, false, false
	}
	if err != nil {
		log.Printf("read product cache failed: %v", err)
		return nil, false, false
	}
	if val == stock.NullProductCacheValue {
		return nil, false, true
	}

	var product pb.GetProductResponse
	if err := json.Unmarshal([]byte(val), &product); err != nil {
		_ = s.rdb.Del(ctx, cacheKey).Err()
		return nil, false, false
	}
	return &product, true, false
}

// writeProductCache 将商品信息序列化后写入 Redis 缓存
func (s *ProductService) writeProductCache(ctx context.Context, cacheKey string, product *pb.GetProductResponse) {
	jsonBytes, err := json.Marshal(product)
	if err != nil {
		return
	}
	_ = s.rdb.Set(ctx, cacheKey, jsonBytes, stock.ProductCacheTTL).Err()
}

var ErrProductNotFound = errors.New("product not found")
var ErrStockDeductionLogNotFound = errors.New("stock deduction log not found")
