package main

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/handlers"
	"github.com/sulavmhrzn/choto/internal/middleware"

	ginSwagger "github.com/swaggo/gin-swagger"

	swaggerFiles "github.com/swaggo/files"
)

func NewRouter(rdb *redis.Client, h *handlers.Handler, cfg *config.Config) *gin.Engine {
	router := gin.Default()
	router.Use(cors.Default())

	v1 := router.Group("/api/v1")
	v1.Use(middleware.RateLimiter(rdb, 100, time.Minute))
	{
		api := v1.Group("/")
		api.Use(middleware.IsAuthenticated(h.Service.UserService))
		{
			api.GET("/auth/me", h.GetMe)
			api.GET("/auth/dashboard", h.GetDashboard)
			api.POST("/urls/shorten", h.Shorten)
			api.GET("/urls/stats/:code", h.GetStats)
			api.DELETE("/urls/:code", h.DeleteURL)
			api.GET("/urls/:code/qr", h.GenerateQRCode)
			api.GET("/clicks/:code", h.GetURLClicks)
			api.GET("/clicks/:code/stats", h.GetURLClicksStat)
		}

		auth := v1.Group("/auth")
		{
			auth.POST("/register", h.Register)
			auth.POST("/login", h.Login)
			auth.POST("/refresh", h.Refresh)
		}

		v1.GET("/ping", h.Ping)
		v1.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}
	router.GET("/:code", h.Redirect)

	return router
}
