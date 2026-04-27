package grpc

import (
	"context"
	"errors"
	"log"

	pb "micro-proto"
	"user-service/internal/service"
)

type Handler struct {
	pb.UnimplementedUserServiceServer
	userService *service.UserService
}

func NewHandler(userService *service.UserService) *Handler {
	return &Handler{userService: userService}
}

func (h *Handler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	log.Printf("register request: username=%s", req.Username)

	user, err := h.userService.Register(ctx, req.Username, req.Password)
	if err != nil {
		log.Printf("register failed: username=%s err=%v", req.Username, err)
		return &pb.RegisterResponse{
			Code:    500,
			Message: "register failed, username may already exist",
		}, nil
	}

	return &pb.RegisterResponse{
		Code:    200,
		Message: "register success",
		UserId:  int32(user.ID),
	}, nil
}

func (h *Handler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	log.Printf("login request: username=%s", req.Username)

	user, err := h.userService.Login(ctx, req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			return &pb.LoginResponse{Code: 404, Message: "user not found"}, nil
		case errors.Is(err, service.ErrInvalidPassword):
			return &pb.LoginResponse{Code: 401, Message: "invalid password"}, nil
		default:
			log.Printf("login failed: username=%s err=%v", req.Username, err)
			return &pb.LoginResponse{Code: 500, Message: "login failed"}, nil
		}
	}

	return &pb.LoginResponse{
		Code:    200,
		Message: "login success",
		UserId:  int32(user.ID),
	}, nil
}
