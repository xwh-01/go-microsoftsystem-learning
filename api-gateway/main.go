package main

import (
	"log"

	"api-gateway/internal/auth"
	"api-gateway/internal/httpapi"
	pb "micro-proto"

	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func dialGRPC(target string) (*grpc.ClientConn, error) {
	return grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func main() {
	viper.SetConfigFile("../config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("read config failed: %v", err)
	}

	userConn, err := dialGRPC(viper.GetString("user_service.target"))
	if err != nil {
		log.Fatalf("connect user service failed: %v", err)
	}
	defer userConn.Close()

	productConn, err := dialGRPC(viper.GetString("product_service.target"))
	if err != nil {
		log.Fatalf("connect product service failed: %v", err)
	}
	defer productConn.Close()

	orderConn, err := dialGRPC(viper.GetString("order_service.target"))
	if err != nil {
		log.Fatalf("connect order service failed: %v", err)
	}
	defer orderConn.Close()

	jwtManager := auth.NewJWTManager(viper.GetString("api_gateway.jwt_secret"))
	router := httpapi.NewRouter(
		httpapi.Clients{
			User:    pb.NewUserServiceClient(userConn),
			Product: pb.NewProductServiceClient(productConn),
			Order:   pb.NewOrderServiceClient(orderConn),
		},
		jwtManager.Middleware(),
		jwtManager.GenerateToken,
	)

	port := viper.GetString("api_gateway.port")
	log.Printf("api gateway started on %s", port)
	if err := router.Run(port); err != nil {
		log.Fatalf("api gateway failed: %v", err)
	}
}
