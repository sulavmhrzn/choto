package service

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	ErrAccessTokenExpired  = errors.New("access token expired")
	ErrInvalidToken        = errors.New("invalid token")
	ErrUserNotFound        = errors.New("user not found")
)

type Token struct {
	AccessToken  string
	RefreshToken string
}
type Authenticator interface {
	CreateUser(ctx context.Context, email string, password string) (*repository.User, error)
	Login(ctx context.Context, email string, passwordHash string) (*Token, error)
	Refresh(ctx context.Context, oldRefreshToken string) (string, error)
	VerifyAccessToken(tokenString string) (jwt.MapClaims, error)
	GetUserByID(ctx context.Context, id int64) (*repository.User, error)
	GetUserDashboard(ctx context.Context, userID int64) (*repository.Dashboard, error)
}

type UserRepository interface {
	Create(ctx context.Context, email string, passwordHash string) (*repository.User, error)
	GetByEmail(ctx context.Context, email string) (*repository.User, error)
	GetByID(ctx context.Context, id int64) (*repository.User, error)
	GetDashboard(ctx context.Context, userID int64) (*repository.Dashboard, error)
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

func (s *UserService) comparePassword(rawPassword, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(rawPassword))
	return err == nil

}

func (s *UserService) generateAccessToken(userID int64) (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    "choto-api",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.SecretKey))
}

func (s *UserService) VerifyAccessToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			log.Printf("error verifying signing method")
			return nil, ErrInvalidToken
		}
		return []byte(s.config.SecretKey), nil
	})
	if err != nil {
		log.Printf("[service] error verifying token: %v", err)
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrAccessTokenExpired
		}
		return nil, ErrInvalidToken
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, ErrInvalidToken
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
	if !s.comparePassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	accessToken, err := s.generateAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
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
	return s.generateAccessToken(userID)
}

func (s *UserService) GetUserByID(ctx context.Context, id int64) (*repository.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (s *UserService) GetUserDashboard(ctx context.Context, userID int64) (*repository.Dashboard, error) {
	data, err := s.repo.GetDashboard(ctx, userID)
	if err != nil {
		return nil, err
	}
	return data, nil
}
