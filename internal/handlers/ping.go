package handlers

import (
	"net/http"
	"runtime"
	"time"

	_ "github.com/sulavmhrzn/choto/docs"

	"github.com/gin-gonic/gin"
)

type SystemStats struct {
	MemoryUsageMB uint64 `json:"memory_usage_mb" example:"24"`
	Goroutines    int    `json:"goroutines" example:"8"`
}
type PingResponse struct {
	Status      string      `json:"status" example:"up"`
	Environment string      `json:"environment" example:"development"`
	Version     string      `json:"version" example:"1.0.0"`
	Uptime      string      `json:"uptime" example:"20s"`
	System      SystemStats `json:"system"`
	Timestamp   string      `json:"timestamp" example:"2026-01-14T14:20:42+05:45"`
}

// Ping godoc
// @Summary      Health Check
// @Description  Returns server status, environment details, and system metrics like memory usage.
// @Tags         system
// @Produce      json
// @Success      200 {object} PingResponse
// @Router       /ping [get]
func (h *Handler) Ping(c *gin.Context) {
	uptime := time.Since(h.StartTime).Round(time.Second).String()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, PingResponse{
		Status:      "up",
		Environment: h.Config.Env,
		Version:     "1.0.0",
		Uptime:      uptime,
		System: SystemStats{
			MemoryUsageMB: m.Alloc / 1024 / 1024,
			Goroutines:    runtime.NumGoroutine(),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	})
}
