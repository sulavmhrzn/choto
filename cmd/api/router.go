package main

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/handlers"
	"github.com/sulavmhrzn/choto/internal/middleware"
)

func NewRouter(rdb *redis.Client, h *handlers.Handler, cfg *config.Config) *gin.Engine {
	router := gin.Default()
	router.GET("/ping", h.Ping)
	protected := router.Group("/")
	protected.Use(middleware.RateLimiter(rdb, cfg.RateLimitCount, time.Minute))
	{
		protected.POST("/shorten", h.Shorten)
	}

	router.GET("/:code", h.Redirect)
	router.GET("/stats/:code", h.GetStats)
	router.POST("/auth/register", h.Register)
	router.POST("/auth/login", h.Login)
	router.POST("/auth/refresh", h.Refresh)
	return router
}
