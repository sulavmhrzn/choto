package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Ping(c *gin.Context) {
	uptime := time.Since(h.StartTime).Round(time.Second).String()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, gin.H{
		"status":      "up",
		"environment": h.Config.Env,
		"version":     "1.0.0",
		"uptime":      uptime,
		"system": gin.H{
			"memory_usage_mb": m.Alloc / 1024 / 1024,
			"goroutines":      runtime.NumGoroutine(),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
