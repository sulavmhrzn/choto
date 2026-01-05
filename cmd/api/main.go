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

	h := handlers.NewHandler(logger, cfg, startTime)

	_, err = storage.NewPostgres(cfg.DBConn)
	if err != nil {
		logger.Error("failed to connect to the database", "err", err)
		os.Exit(1)
	}

	r := gin.Default()
	r.GET("/ping", h.Ping)

	r.Run(fmt.Sprintf(":%s", cfg.ServerPort))
}
