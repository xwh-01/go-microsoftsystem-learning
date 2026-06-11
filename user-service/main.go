package main

import (
	"log"
	"net"

	pb "micro-proto"
	usergrpc "user-service/internal/grpc"
	"user-service/internal/repository"
	"user-service/internal/service"

	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// user-service 用户服务主入口
// 负责用户注册与登录，对外通过 gRPC 暴露 UserService
func main() {
	viper.SetConfigFile("../config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("read config failed: %v", err)
	}

	db, err := gorm.Open(mysql.Open(viper.GetString("mysql_dsn")), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect MySQL failed: %v", err)
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

	log.Printf("user service started on %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve failed: %v", err)
	}
}
