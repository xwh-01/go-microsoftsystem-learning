package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims 自定义 JWT 声明，嵌入用户ID
type Claims struct {
	UserID int32 `json:"user_id"`
	jwt.RegisteredClaims
}

// JWTManager 管理 JWT 令牌的生成与验证
type JWTManager struct {
	secret []byte
}

// NewJWTManager 创建 JWT 管理器，secret 为签名密钥
func NewJWTManager(secret string) *JWTManager {
	return &JWTManager{secret: []byte(secret)}
}

// GenerateToken 为用户生成 HS256 签名的 JWT 令牌，有效期 24 小时
func (m *JWTManager) GenerateToken(userID int32) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			Issuer:    "api-gateway",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Middleware 返回 Gin 中间件：验证 Authorization 头中的 Bearer 令牌，解析并注入 userID 到上下文
func (m *JWTManager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing Authorization header"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token format, expected Bearer <token>"})
			c.Abort()
			return
		}

		token, err := jwt.ParseWithClaims(parts[1], &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return m.secret, nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Next()
	}
}
