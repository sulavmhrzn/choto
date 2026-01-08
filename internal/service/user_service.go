package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sulavmhrzn/choto/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyInUse = errors.New("email is already in use")
)

type Authenticator interface {
	CreateUser(ctx context.Context, email string, password string) (*repository.User, error)
}

type UserRepository interface {
	Create(ctx context.Context, email string, passwordHash string) (*repository.User, error)
}

type UserService struct {
	repo UserRepository
}

func NewUserService(repo UserRepository) Authenticator {
	return &UserService{
		repo: repo,
	}
}

func (s *UserService) hashPassword(password string) (string, error) {
	p, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(p), err
}

func (s *UserService) CreateUser(ctx context.Context, email string, password string) (*repository.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	hashedPassword, err := s.hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("error while hashing password %v", err)
	}
	user, err := s.repo.Create(ctx, cleanEmail, hashedPassword)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return nil, ErrEmailAlreadyInUse
		}
		return nil, err
	}
	return user, nil
}
