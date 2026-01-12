package handlers

import (
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/service"
)

type Handler struct {
	Logger    *slog.Logger
	Config    *config.Config
	StartTime time.Time
	RDB       *redis.Client
	Service   *service.Service
}

func NewHandler(
	logger *slog.Logger,
	cfg *config.Config,
	startTime time.Time,
	rdb *redis.Client,
	service *service.Service,
) *Handler {
	return &Handler{
		Logger:    logger,
		Config:    cfg,
		StartTime: startTime,
		RDB:       rdb,
		Service:   service,
	}
}
