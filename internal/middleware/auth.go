package middleware

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sulavmhrzn/choto/internal/service"
)

func IsAuthenticated(userService service.Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Request.Header
		authorizationHeader := headers.Get("Authorization")
		if authorizationHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Requires authentication token"})
			c.Abort()
			return
		}
		parts := strings.Split(authorizationHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Requires authentication token"})
			c.Abort()
			return
		}
		token := parts[1]
		claims, err := userService.VerifyAccessToken(token)
		if err != nil {
			log.Printf("error verifying access token: %v", err)
			switch {
			case errors.Is(err, service.ErrInvalidToken):
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				c.Abort()
			case errors.Is(err, service.ErrAccessTokenExpired):
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Access token expired"})
				c.Abort()
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			}
			return
		}
		userID, err := claims.GetSubject()
		if err != nil {
			log.Printf("error getting subject: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			c.Abort()
			return
		}
		id, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			log.Printf("cannot convert userID `%s` to number.", userID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			c.Abort()
			return
		}
		c.Set("user_id", id)
		c.Next()
	}
}
