package service

import (
	"database/sql"

	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/repository"
)

type Service struct {
	UserService Authenticator
	URLService  Shortener
}

func NewService(db *sql.DB, rdb *redis.Client, config *config.Config) *Service {
	return &Service{
		UserService: NewUserService(repository.NewUserRepository(db), config, rdb),
		URLService:  NewURLService(repository.NewURLRepository(db, rdb, config), rdb),
	}
}
