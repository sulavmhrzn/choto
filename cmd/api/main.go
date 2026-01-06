package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lmittmann/tint"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/handlers"
	"github.com/sulavmhrzn/choto/internal/repository"
	"github.com/sulavmhrzn/choto/internal/service"
	"github.com/sulavmhrzn/choto/internal/storage"
)

func main() {
	startTime := time.Now()
	logger := slog.New(tint.NewHandler(os.Stdout, nil))

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	db, err := storage.NewPostgres(cfg.DBConn)
	if err != nil {
		logger.Error("failed to connect to the database", "err", err)
		os.Exit(1)
	}

	rdb, err := storage.NewRedis(cfg.RedisAddr)
	if err != nil {
		logger.Error("failed to connect to redis client", "err", err)
		os.Exit(1)
	}

	urlRepo := repository.NewURLRepository(db, rdb, cfg)
	urlService := service.NewURLService(urlRepo)
	h := handlers.NewHandler(logger, cfg, startTime, urlService)

	r := gin.Default()
	r.GET("/ping", h.Ping)
	r.POST("/shorten", h.Shorten)
	r.GET("/:code", h.Redirect)
	r.GET("/stats/:code", h.GetStats)

	r.Run(fmt.Sprintf(":%s", cfg.ServerPort))
}
