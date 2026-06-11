package service

import (
	"context"
	"errors"
	"fmt"

	"user-service/internal/model"
	"user-service/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

// 业务层错误定义
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidPassword = errors.New("invalid password")

// UserService 用户业务逻辑层
type UserService struct {
	repo userRepository
}

// userRepository 用户仓储接口，解耦具体数据库实现
type userRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByUsername(ctx context.Context, username string) (*model.User, error)
}

// NewUserService 创建用户服务实例
func NewUserService(repo userRepository) *UserService {
	return &UserService{repo: repo}
}

// Register 注册新用户：bcrypt 加密密码后写入数据库
func (s *UserService) Register(ctx context.Context, username string, password string) (*model.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("encrypt password failed: %w", err)
	}

	user := &model.User{
		Username: username,
		Password: string(hashedPassword),
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user failed: %w", err)
	}
	return user, nil
}

// Login 用户登录：查询用户后比对 bcrypt 密码哈希
func (s *UserService) Login(ctx context.Context, username string, password string) (*model.User, error) {
	user, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user failed: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}
	return user, nil
}
