package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/models"
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
	CreateUser(ctx context.Context, email string, password string) (*models.User, error)
	Login(ctx context.Context, email string, passwordHash string) (*Token, error)
	Refresh(ctx context.Context, oldRefreshToken string) (string, error)
	VerifyAccessToken(ctx context.Context, tokenString string) (jwt.MapClaims, error)
	GetUserByID(ctx context.Context, id int64) (*models.User, error)
	GetUserDashboard(ctx context.Context, userID int64) (*models.Dashboard, error)
}

type UserRepository interface {
	Create(ctx context.Context, email string, passwordHash string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByID(ctx context.Context, id int64) (*models.User, error)
	GetDashboard(ctx context.Context, userID int64) (*models.Dashboard, error)
}

type UserService struct {
	repo   UserRepository
	config *config.Config
	rdb    *redis.Client
	logger *slog.Logger
}

func NewUserService(repo UserRepository, config *config.Config, rdb *redis.Client, logger *slog.Logger) Authenticator {
	return &UserService{
		repo:   repo,
		config: config,
		rdb:    rdb,
		logger: logger,
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

func (s *UserService) VerifyAccessToken(ctx context.Context, tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			s.logger.WarnContext(ctx, "invalid token signing method",
				slog.String("alg", t.Header["alg"].(string)),
			)
			return nil, ErrInvalidToken
		}
		return []byte(s.config.SecretKey), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			s.logger.DebugContext(ctx, "token expired")
			return nil, ErrAccessTokenExpired
		}

		s.logger.WarnContext(ctx, "token verification failed",
			slog.Any("error", err),
		)
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID := claims["sub"]
		s.logger.DebugContext(ctx, "token verified", slog.Any("user_id", userID))
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func (s *UserService) CreateUser(ctx context.Context, email string, password string) (*models.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	logger := s.logger.With(slog.String("email", cleanEmail))

	hashedPassword, err := s.hashPassword(password)
	if err != nil {
		logger.ErrorContext(ctx, "failed to hash password", slog.Any("error", err))
		return nil, fmt.Errorf("error while hashing password %w", err)
	}

	user, err := s.repo.Create(ctx, cleanEmail, hashedPassword)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			logger.WarnContext(ctx, "user registration failed: duplicate email")
			return nil, ErrEmailAlreadyInUse
		}

		logger.ErrorContext(ctx, "repository failed to create user", slog.Any("error", err))
		return nil, err
	}

	logger.InfoContext(ctx, "user created successfully", slog.Int64("user_id", user.ID))
	return user, nil
}

func (s *UserService) Login(ctx context.Context, email, password string) (*Token, error) {
	logger := s.logger.With(slog.String("email", email))

	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			logger.WarnContext(ctx, "login failed: user not found")
			return nil, ErrInvalidCredentials
		}
		logger.ErrorContext(ctx, "database error during login", slog.Any("error", err))
		return nil, err
	}

	if !s.comparePassword(password, user.PasswordHash) {
		logger.WarnContext(ctx, "login failed: incorrect password",
			slog.Int64("user_id", user.ID),
		)
		return nil, ErrInvalidCredentials
	}

	accessToken, err := s.generateAccessToken(user.ID)
	if err != nil {
		logger.ErrorContext(ctx, "access token generation failed",
			slog.Int64("user_id", user.ID),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken := uuid.New().String()
	err = s.rdb.Set(ctx, "refresh:"+refreshToken, user.ID, 7*24*time.Hour).Err()
	if err != nil {
		logger.ErrorContext(ctx, "redis failure: failed to store refresh token",
			slog.Int64("user_id", user.ID),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("failed to set refresh token in redis client: %w", err)
	}

	logger.InfoContext(ctx, "user logged in successfully",
		slog.Int64("user_id", user.ID),
	)

	return &Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *UserService) Refresh(ctx context.Context, oldRefreshToken string) (string, error) {
	userID, err := s.rdb.Get(ctx, "refresh:"+oldRefreshToken).Int64()
	if err != nil {
		s.logger.WarnContext(ctx, "refresh token invalid or expired",
			slog.Any("error", err),
		)
		return "", ErrRefreshTokenExpired
	}

	accessToken, err := s.generateAccessToken(userID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to generate access token during refresh",
			slog.Int64("user_id", userID),
			slog.Any("error", err),
		)
		return "", err
	}

	s.logger.InfoContext(ctx, "token refreshed successfully", slog.Int64("user_id", userID))
	return accessToken, nil
}

func (s *UserService) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			s.logger.DebugContext(ctx, "user fetch: not found", slog.Int64("user_id", id))
			return nil, ErrUserNotFound
		}

		s.logger.ErrorContext(ctx, "failed to fetch user by id",
			slog.Int64("user_id", id),
			slog.Any("error", err),
		)
		return nil, err
	}
	return user, nil
}

func (s *UserService) GetUserDashboard(ctx context.Context, userID int64) (*models.Dashboard, error) {
	data, err := s.repo.GetDashboard(ctx, userID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get user dashboard",
			slog.Int64("user_id", userID),
			slog.Any("error", err),
		)
		return nil, err
	}
	return data, nil
}
