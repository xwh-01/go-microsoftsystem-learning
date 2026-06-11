package repository

import (
	"context"
	"errors"

	"user-service/internal/model"

	"gorm.io/gorm"
)

var ErrUserNotFound = errors.New("user not found")

// UserRepository 用户数据访问层，封装 GORM 数据库操作
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储实例
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// AutoMigrate 自动迁移 User 表结构
func (r *UserRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&model.User{})
}

// Create 创建新用户记录
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// FindByUsername 根据用户名查询用户，未找到返回 ErrUserNotFound
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}
