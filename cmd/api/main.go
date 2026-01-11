package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	urlService := service.NewURLService(urlRepo, rdb)
	userRepo := repository.NewUserRepository(db)
	userService := service.NewUserService(userRepo, cfg, rdb)
	handlers := handlers.NewHandler(logger, cfg, startTime, urlService, userService, rdb)

	router := NewRouter(rdb, handlers, cfg)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.ServerPort),
		Handler: router,
	}

	cleanupContext, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go urlService.StartCleanupWorker(cleanupContext, 1*time.Minute)

	go func() {
		logger.Info("Server starting", "port", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("failed to shutdown", "err", err)
		os.Exit(1)
	}
	logger.Info("Closing database and redis connections...")
	cleanupCancel()
	db.Close()
	rdb.Close()
	logger.Info("Server exited gracefully")

}
