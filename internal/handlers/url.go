package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sulavmhrzn/choto/internal/service"
)

func (h *Handler) Shorten(c *gin.Context) {
	var req struct {
		URL       string `json:"url" binding:"required,url"`
		ExpiresIn int    `json:"expires_in_hours" binding:"omitempty,number,gt=0"`
		Alias     string `json:"alias" binding:"omitempty,alphanum,min=3,max=20"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour)
		expiresAt = &t
	}
	userID := c.GetInt64("user_id")
	url, err := h.Service.URLService.Shorten(c.Request.Context(), req.URL, expiresAt, req.Alias, userID)
	if err != nil {
		h.Logger.Error("failed to shorten", "user_id", userID, "err", err)
		switch {
		case errors.Is(err, service.ErrInvalidScheme):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Accepts only http/https schemes"})
		case errors.Is(err, service.ErrAliasAlreadyTaken):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Alias already taken"})
		case errors.Is(err, service.ErrReservedAlias):
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%q is not allowed as an alias", req.Alias)})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}
	val, exists := c.Get("rate_limit_count")
	if exists {
		count := val.(int64)
		c.Header("X-RateLimit-Limit", strconv.FormatInt(int64(h.Config.RateLimitCount), 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(int64(h.Config.RateLimitCount)-count, 10))
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":         url.ID,
		"short_code": url.ShortCode,
		"long_url":   url.LongURL,
		"clicks":     url.Clicks,
		"short_url":  h.Config.BaseURL + "/" + url.ShortCode,
		"expires_at": url.ExpiresAt,
		"created_at": url.CreatedAt,
	})
}

func (h *Handler) Redirect(c *gin.Context) {
	code := c.Param("code")
	longURL, err := h.Service.URLService.GetOriginalURL(c.Request.Context(), code)
	if err != nil {
		h.Logger.Warn("redirect failed", "code", code, "err", err)
		switch {
		case errors.Is(err, service.ErrShortCodeRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Short code required"})
		case errors.Is(err, service.ErrURLNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Short link not found"})
		case errors.Is(err, service.ErrShortCodeExpired):
			c.JSON(http.StatusGone, gin.H{"error": "Short link has expired"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}
	h.Service.URLService.TrackClick(code)
	c.Redirect(http.StatusFound, longURL)
}

func (h *Handler) GetStats(c *gin.Context) {
	code := c.Param("code")
	userID := c.GetInt64("user_id")

	stats, err := h.Service.URLService.GetStats(c.Request.Context(), code, userID)
	if err != nil {
		h.Logger.Warn("stats failed", "code", code, "err", err)
		switch {
		case errors.Is(err, service.ErrURLNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Short link not found"})
		case errors.Is(err, service.ErrShortCodeExpired):
			c.JSON(http.StatusGone, gin.H{"error": "Short link has expired"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) DeleteURL(c *gin.Context) {
	code := c.Param("code")
	userID := c.GetInt64("user_id")

	err := h.Service.URLService.DeleteURL(c.Request.Context(), code, userID)
	if err != nil {
		h.Logger.Error("failed to delete url", "code", code, "err", err)
		if errors.Is(err, service.ErrURLNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.Status(http.StatusNoContent)
}
