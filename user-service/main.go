package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "micro-proto"
	usergrpc "user-service/internal/grpc"
	"user-service/internal/repository"
	"user-service/internal/service"

	"github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	viper.SetConfigFile("../config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("read config failed: %v", err)
	}

	db, err := gorm.Open(sqlite.Open("user.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect database failed: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	if err := userRepo.AutoMigrate(); err != nil {
		log.Fatalf("migrate database failed: %v", err)
	}

	userService := service.NewUserService(userRepo)

	port := viper.GetString("user_service.port")
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterUserServiceServer(grpcServer, usergrpc.NewHandler(userService))

	consulClient, err := api.NewClient(&api.Config{Address: viper.GetString("common.consul_addr")})
	if err != nil {
		log.Fatalf("create consul client failed: %v", err)
	}
	registration := &api.AgentServiceRegistration{
		ID:      viper.GetString("user_service.node_id"),
		Name:    viper.GetString("user_service.name"),
		Port:    50051,
		Address: "127.0.0.1",
	}
	if err := consulClient.Agent().ServiceRegister(registration); err != nil {
		log.Fatalf("register consul service failed: %v", err)
	}

	log.Printf("user service started on %s", port)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("serve failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	if err := consulClient.Agent().ServiceDeregister(viper.GetString("user_service.node_id")); err != nil {
		log.Printf("deregister consul service failed: %v", err)
	}
	grpcServer.GracefulStop()
	log.Println("user service stopped")
}
