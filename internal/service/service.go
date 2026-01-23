package service

import (
	"database/sql"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/repository"
)

type Service struct {
	UserService Authenticator
	URLService  Shortener
}

func NewService(db *sql.DB, rdb *redis.Client, config *config.Config, logger *slog.Logger) *Service {
	userRepo := repository.NewUserRepository(db, logger)
	urlRepo := repository.NewURLRepository(db, rdb, config, logger)

	return &Service{
		UserService: NewUserService(userRepo, config, rdb, logger),
		URLService:  NewURLService(urlRepo, rdb, logger, config),
	}
}
