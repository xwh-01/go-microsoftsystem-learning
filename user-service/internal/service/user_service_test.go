package service

import (
	"context"
	"errors"
	"testing"

	"user-service/internal/model"
	"user-service/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type stubUserRepository struct {
	createFn         func(ctx context.Context, user *model.User) error
	findByUsernameFn func(ctx context.Context, username string) (*model.User, error)
}

func (s *stubUserRepository) Create(ctx context.Context, user *model.User) error {
	return s.createFn(ctx, user)
}

func (s *stubUserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	return s.findByUsernameFn(ctx, username)
}

func TestRegisterHashesPassword(t *testing.T) {
	var savedUser *model.User
	repo := &stubUserRepository{
		createFn: func(ctx context.Context, user *model.User) error {
			savedUser = &model.User{
				Username: user.Username,
				Password: user.Password,
			}
			return nil
		},
	}

	svc := NewUserService(repo)
	user, err := svc.Register(context.Background(), "alice", "123456")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("expected username alice, got %s", user.Username)
	}
	if savedUser == nil {
		t.Fatal("expected repository Create to be called")
	}
	if savedUser.Password == "123456" {
		t.Fatal("expected password to be hashed, got plaintext")
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to create hashed password: %v", err)
	}

	repo := &stubUserRepository{
		findByUsernameFn: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{
				Username: username,
				Password: string(hashedPassword),
			}, nil
		},
	}

	svc := NewUserService(repo)
	_, err = svc.Login(context.Background(), "alice", "wrong-password")
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	repo := &stubUserRepository{
		findByUsernameFn: func(ctx context.Context, username string) (*model.User, error) {
			return nil, repository.ErrUserNotFound
		},
	}

	svc := NewUserService(repo)
	_, err := svc.Login(context.Background(), "missing", "123456")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}
