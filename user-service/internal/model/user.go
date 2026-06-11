package model

import "gorm.io/gorm"

// User 用户表模型，username 唯一索引，password 存储 bcrypt 哈希
type User struct {
	gorm.Model
	Username string `gorm:"unique;not null"`
	Password string `gorm:"not null"`
}
