package handlers

import (
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/service"
)

type Handler struct {
	Logger     *slog.Logger
	Config     *config.Config
	StartTime  time.Time
	URLService service.Shortener
	RDB        *redis.Client
}

func NewHandler(logger *slog.Logger, cfg *config.Config, startTime time.Time, urlService service.Shortener, rdb *redis.Client) *Handler {
	return &Handler{
		Logger:     logger,
		Config:     cfg,
		StartTime:  startTime,
		URLService: urlService,
		RDB:        rdb,
	}
}
