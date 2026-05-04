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

type productRepository interface {
	Create(ctx context.Context, product *model.Product) error
	FindByID(ctx context.Context, productID int32) (*model.Product, error)
	DeductStock(ctx context.Context, productID int32, quantity int32) (bool, error)
}

type ProductService struct {
	repo productRepository
	rdb  *redis.Client
}

func NewProductService(repo productRepository, rdb *redis.Client) *ProductService {
	return &ProductService{
		repo: repo,
		rdb:  rdb,
	}
}

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

func (s *ProductService) writeProductCache(ctx context.Context, cacheKey string, product *pb.GetProductResponse) {
	jsonBytes, err := json.Marshal(product)
	if err != nil {
		return
	}
	_ = s.rdb.Set(ctx, cacheKey, jsonBytes, stock.ProductCacheTTL).Err()
}

var ErrProductNotFound = errors.New("product not found")
