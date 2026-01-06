package middleware

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RateLimiter(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		key := fmt.Sprintf("rate:%s", ctx.ClientIP())
		count, err := rdb.Incr(ctx.Request.Context(), key).Result()
		if err != nil {
			log.Printf("failed to increment rate limit count for %s: %v", key, err)
			ctx.Next()
			return
		}
		if count == 1 {
			rdb.Expire(ctx.Request.Context(), key, window)
		}

		if count > int64(limit) {
			ctx.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests.",
			})
			ctx.Abort()
			return
		}
		ctx.Set("rate_limit_count", count)
		ctx.Next()
	}
}
