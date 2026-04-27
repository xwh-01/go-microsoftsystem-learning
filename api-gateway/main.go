package main

import (
	"log"

	"api-gateway/internal/auth"
	"api-gateway/internal/discovery"
	"api-gateway/internal/httpapi"
	pb "micro-proto"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func initSentinel() {
	if err := sentinel.InitDefault(); err != nil {
		log.Fatalf("init sentinel failed: %v", err)
	}

	_, err := flow.LoadRules([]*flow.Rule{
		{
			Resource:               "create_order",
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			Threshold:              10,
			StatIntervalInMs:       1000,
		},
	})
	if err != nil {
		log.Fatalf("load flow rules failed: %v", err)
	}
}

func dialGRPC(target string) (*grpc.ClientConn, error) {
	return grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func main() {
	viper.SetConfigFile("../config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("read config failed: %v", err)
	}

	initSentinel()

	consulAddr := viper.GetString("common.consul_addr")
	userTarget, err := discovery.DiscoverService(consulAddr, viper.GetString("user_service.name"))
	if err != nil {
		log.Fatalf("discover user service failed: %v", err)
	}
	productTarget, err := discovery.DiscoverService(consulAddr, viper.GetString("product_service.name"))
	if err != nil {
		log.Fatalf("discover product service failed: %v", err)
	}
	orderTarget, err := discovery.DiscoverService(consulAddr, viper.GetString("order_service.name"))
	if err != nil {
		log.Fatalf("discover order service failed: %v", err)
	}

	userConn, err := dialGRPC(userTarget)
	if err != nil {
		log.Fatalf("connect user service failed: %v", err)
	}
	defer userConn.Close()

	productConn, err := dialGRPC(productTarget)
	if err != nil {
		log.Fatalf("connect product service failed: %v", err)
	}
	defer productConn.Close()

	orderConn, err := dialGRPC(orderTarget)
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
