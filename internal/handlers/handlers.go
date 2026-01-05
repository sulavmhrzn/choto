package handlers

import (
	"log/slog"
	"time"

	"github.com/sulavmhrzn/choto/internal/config"
)

type Handler struct {
	Logger    *slog.Logger
	Config    *config.Config
	StartTime time.Time
}

func NewHandler(logger *slog.Logger, cfg *config.Config, startTime time.Time) *Handler {
	return &Handler{
		Logger:    logger,
		Config:    cfg,
		StartTime: startTime,
	}
}
