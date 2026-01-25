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
	"github.com/sulavmhrzn/choto/docs"
	_ "github.com/sulavmhrzn/choto/docs"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/handlers"
	"github.com/sulavmhrzn/choto/internal/service"
	"github.com/sulavmhrzn/choto/internal/storage"
)

// @title           Choto URL Shortener API
// @version         1.0
// @description     A high-performance URL shortener service with analytics and QR code generation.
// @termsOfService  https://choto.np/terms/

// @contact.name   Choto Support
// @contact.url    https://github.com/your-username/choto
// @contact.email  support@choto.np

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in                         header
// @name                       Authorization
// @description                Type 'Bearer ' followed by your JWT token. Example: "Bearer eyJhbG..."

// @externalDocs.description  Choto Project Documentation
// @externalDocs.url          https://github.com/sulavmhrzn/choto#readme
func main() {
	startTime := time.Now()
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

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

	svc := service.NewService(db, rdb, cfg, logger)
	handlers := handlers.NewHandler(logger, cfg, startTime, rdb, svc)

	router := NewRouter(rdb, handlers, cfg)
	docs.SwaggerInfo.Host = fmt.Sprintf("localhost:%s", cfg.ServerPort)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.ServerPort),
		Handler: router,
	}

	cleanupContext, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go svc.URLService.StartCleanupWorker(cleanupContext, time.Duration(cfg.OldLinksCleanUpIntervalInHours)*time.Hour)

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
	svc.URLService.StopClickCollector()
	svc.URLService.WaitClickCollector()
	cleanupCancel()
	db.Close()
	rdb.Close()
	logger.Info("Server exited gracefully")

}
