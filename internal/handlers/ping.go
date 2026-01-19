package handlers

import (
	"net/http"
	"runtime"
	"time"

	_ "github.com/sulavmhrzn/choto/docs"
	"github.com/sulavmhrzn/choto/internal/dtos"

	"github.com/gin-gonic/gin"
)

// Ping godoc
// @Summary      Health Check
// @Description  Returns server status, environment details, and system metrics like memory usage.
// @Tags         system
// @Produce      json
// @Success      200 {object} dtos.PingResponse
// @Router       /ping [get]
func (h *Handler) Ping(c *gin.Context) {
	uptime := time.Since(h.StartTime).Round(time.Second).String()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, dtos.PingResponse{
		Status:      "up",
		Environment: h.Config.Env,
		Version:     "1.0.0",
		Uptime:      uptime,
		System: dtos.SystemStats{
			MemoryUsageMB: m.Alloc / 1024 / 1024,
			Goroutines:    runtime.NumGoroutine(),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	})
}
