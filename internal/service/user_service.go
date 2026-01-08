package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyInUse   = errors.New("email is already in use")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrRefreshTokenExpired = errors.New("refresh token expired")
)

type Token struct {
	AccessToken  string
	RefreshToken string
}
type Authenticator interface {
	CreateUser(ctx context.Context, email string, password string) (*repository.User, error)
	Login(ctx context.Context, email string, passwordHash string) (*Token, error)
	Refresh(ctx context.Context, oldRefreshToken string) (string, error)
}

type UserRepository interface {
	Create(ctx context.Context, email string, passwordHash string) (*repository.User, error)
	GetByEmail(ctx context.Context, email string) (*repository.User, error)
}

type UserService struct {
	repo   UserRepository
	config *config.Config
	rdb    *redis.Client
}

func NewUserService(repo UserRepository, config *config.Config, rdb *redis.Client) Authenticator {
	return &UserService{
		repo:   repo,
		config: config,
		rdb:    rdb,
	}
}

func (s *UserService) hashPassword(password string) (string, error) {
	p, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(p), err
}

func (s *UserService) ComparePassword(rawPassword, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(rawPassword))
	return err == nil

}

func (s *UserService) GenerateAccessToken(userID int) (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Subject:   strconv.Itoa(userID),
		Issuer:    "choto-api",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.SecretKey))
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

func (s *UserService) Login(ctx context.Context, email, password string) (*Token, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if !s.ComparePassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	accessToken, err := s.GenerateAccessToken(user.ID)
	refreshToken := uuid.New().String()
	err = s.rdb.Set(ctx, "refresh:"+refreshToken, user.ID, 7*24*time.Hour).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to set refresh token in redis client: %w", err)
	}
	return &Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *UserService) Refresh(ctx context.Context, oldRefreshToken string) (string, error) {
	userID, err := s.rdb.Get(ctx, "refresh:"+oldRefreshToken).Int64()
	if err != nil {
		return "", ErrRefreshTokenExpired
	}
	return s.GenerateAccessToken(int(userID))
}
