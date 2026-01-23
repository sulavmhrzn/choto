package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		end := time.Since(start)
		status := c.Writer.Status()

		attributes := []slog.Attr{
			slog.Int("status", status),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.String("query", query),
			slog.String("ip", c.ClientIP()),
			slog.Duration("latency", end),
			slog.String("user-agent", c.Request.UserAgent()),
			slog.String("request_id", requestid.Get(c)),
		}
		if status >= 500 {
			logger.LogAttrs(c.Request.Context(), slog.LevelError, "server error", attributes...)
		} else if status >= 400 {
			logger.LogAttrs(c.Request.Context(), slog.LevelWarn, "client error", attributes...)
		} else {
			logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "request processed", attributes...)
		}
	}
}
