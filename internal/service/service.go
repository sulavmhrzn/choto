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
	return &Service{
		UserService: NewUserService(repository.NewUserRepository(db), config, rdb),
		URLService:  NewURLService(repository.NewURLRepository(db, rdb, config), rdb, logger),
	}
}
