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

	v1 := router.Group("/api/v1")
	v1.Use(middleware.RateLimiter(rdb, 100, time.Minute))
	{
		api := v1.Group("/")
		api.Use(middleware.IsAuthenticated(h.Service.UserService))
		{
			api.POST("/shorten", h.Shorten)
			api.GET("/auth/me", h.GetMe)
			api.GET("/auth/dashboard", h.GetDashboard)
			api.GET("/stats/:code", h.GetStats)
			api.DELETE("/urls/:code", h.DeleteURL)
			api.GET("/urls/:code/qr", h.GenerateQRCode)
			api.GET("/clicks/:code", h.GetURLClicks)
			api.GET("/clicks/stats/:code", h.GetURLClicksStat)
		}

		auth := v1.Group("/auth")
		{
			auth.POST("/register", h.Register)
			auth.POST("/login", h.Login)
			auth.POST("/refresh", h.Refresh)
		}

		v1.GET("/ping", h.Ping)
	}
	router.GET("/:code", h.Redirect)

	return router
}
