package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sulavmhrzn/choto/internal/service"
)

func (h *Handler) Shorten(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required,url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL provided"})
		return
	}
	code, err := h.URLService.Shorten(c.Request.Context(), req.URL)
	if err != nil {
		if errors.Is(err, service.ErrInvalidScheme) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Accepts only http/https schemes"})
			return
		}
		h.Logger.Error("failed to shorted", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"short_code": code,
		"short_url":  h.Config.BaseURL + "/" + code,
	})
}

func (h *Handler) Redirect(c *gin.Context) {
	code := c.Param("code")
	longURL, err := h.URLService.GetOriginalURL(c.Request.Context(), code)
	if err != nil {
		h.Logger.Warn("redirect failed", "code", code, "err", err)
		switch {
		case errors.Is(err, service.ErrShortCodeRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Short code required"})
		case errors.Is(err, service.ErrURLNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Short link not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}
	c.Redirect(http.StatusFound, longURL)
}
