package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "micro-proto"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LoginRequest 登录/注册请求体
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// CreateOrderRequest 下单请求体
type CreateOrderRequest struct {
	ProductID int32 `json:"product_id" binding:"required,gt=0"`
}

// Clients 封装三个后端 gRPC 服务的客户端
type Clients struct {
	User    pb.UserServiceClient
	Product pb.ProductServiceClient
	Order   pb.OrderServiceClient
}

// NewRouter 创建并配置 Gin 路由引擎，注册所有 HTTP 路由
// 路由分为两组：/api/v1/public（无需认证）和 /api/v1/auth（需要 JWT 认证）
func NewRouter(clients Clients, authMiddleware gin.HandlerFunc, generateToken func(int32) (string, error)) *gin.Engine {
	r := gin.Default()
	v1 := r.Group("/api/v1")

	publicGroup := v1.Group("/public")
	{
		publicGroup.POST("/login", func(c *gin.Context) {
			var req LoginRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			grpcRes, err := clients.User.Login(ctx, &pb.LoginRequest{
				Username: req.Username,
				Password: req.Password,
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "user service call failed: " + err.Error()})
				return
			}
			if grpcRes.Code != 200 {
				c.JSON(http.StatusUnauthorized, gin.H{"error": grpcRes.Message})
				return
			}

			token, err := generateToken(grpcRes.UserId)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "generate token failed"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "login success", "token": token})
		})

		publicGroup.POST("/register", func(c *gin.Context) {
			var req LoginRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			grpcRes, err := clients.User.Register(ctx, &pb.RegisterRequest{
				Username: req.Username,
				Password: req.Password,
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "user service call failed: " + err.Error()})
				return
			}
			if grpcRes.Code != 200 {
				c.JSON(http.StatusBadRequest, gin.H{"error": grpcRes.Message})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "register success", "user_id": grpcRes.UserId})
		})

		publicGroup.GET("/product/:id", func(c *gin.Context) {
			productID, err := strconv.Atoi(c.Param("id"))
			if err != nil || productID <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			grpcRes, err := clients.Product.GetProduct(ctx, &pb.GetProductRequest{ProductId: int32(productID)})
			if err != nil {
				if status.Code(err) == codes.NotFound {
					c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "get product failed"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"id":    grpcRes.Id,
				"name":  grpcRes.Name,
				"price": grpcRes.Price,
				"stock": grpcRes.Stock,
			})
		})
	}

	privateGroup := v1.Group("/auth")
	privateGroup.Use(authMiddleware)
	{
		privateGroup.GET("/profile", func(c *gin.Context) {
			userID, _ := c.Get("userID")
			c.JSON(http.StatusOK, gin.H{"message": "profile ok", "user_id": userID})
		})

		privateGroup.POST("/order", func(c *gin.Context) {
			var req CreateOrderRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
				return
			}

			userID, ok := c.Get("userID")
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user id"})
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			grpcRes, err := clients.Order.CreateOrder(ctx, &pb.CreateOrderRequest{
				UserId:    userID.(int32),
				ProductId: req.ProductID,
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "create order failed"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"code":     grpcRes.Code,
				"message":  grpcRes.Message,
				"order_id": grpcRes.OrderId,
			})
		})

		privateGroup.GET("/order/:id", func(c *gin.Context) {
			orderID := strings.TrimSpace(c.Param("id"))
			if orderID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			grpcRes, err := clients.Order.GetOrder(ctx, &pb.GetOrderRequest{OrderId: orderID})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "query order failed"})
				return
			}
			if grpcRes.Code == 404 {
				c.JSON(http.StatusNotFound, gin.H{"error": grpcRes.Message})
				return
			}
			if grpcRes.Code != 200 {
				c.JSON(http.StatusInternalServerError, gin.H{"error": grpcRes.Message})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"order_id":       grpcRes.OrderId,
				"user_id":        grpcRes.UserId,
				"product_id":     grpcRes.ProductId,
				"status":         grpcRes.Status,
				"status_message": grpcRes.StatusMessage,
			})
		})

		privateGroup.GET("/orders/:id/detail", func(c *gin.Context) {
			orderID := strings.TrimSpace(c.Param("id"))
			if orderID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
				return
			}

			userIDValue, ok := c.Get("userID")
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user id"})
				return
			}
			userID := userIDValue.(int32)

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			orderRes, err := clients.Order.GetOrder(ctx, &pb.GetOrderRequest{OrderId: orderID})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "query order failed"})
				return
			}
			if orderRes.Code == 404 {
				c.JSON(http.StatusNotFound, gin.H{"error": orderRes.Message})
				return
			}
			if orderRes.Code != 200 {
				c.JSON(http.StatusInternalServerError, gin.H{"error": orderRes.Message})
				return
			}
			if orderRes.UserId != userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot query another user's order"})
				return
			}

			productRes, err := clients.Product.GetProduct(ctx, &pb.GetProductRequest{ProductId: orderRes.ProductId})
			if err != nil {
				if status.Code(err) == codes.NotFound {
					c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "get product failed"})
				return
			}

			stockResult := gin.H{"status": "pending", "reason": ""}
			stockRes, err := clients.Product.GetStockDeductionLog(ctx, &pb.GetStockDeductionLogRequest{OrderId: orderID})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "query stock result failed"})
				return
			}
			if stockRes.Code == 200 {
				stockResult = gin.H{
					"status": stockRes.Status,
					"reason": stockRes.Reason,
				}
			} else if stockRes.Code != 404 {
				c.JSON(http.StatusInternalServerError, gin.H{"error": stockRes.Message})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"order_id": orderRes.OrderId,
				"user_id":  orderRes.UserId,
				"status":   orderRes.Status,
				"product": gin.H{
					"id":    productRes.Id,
					"name":  productRes.Name,
					"price": productRes.Price,
				},
				"stock_result": stockResult,
				"created_at":   orderRes.CreatedAt,
				"updated_at":   orderRes.UpdatedAt,
			})
		})
	}

	return r
}
