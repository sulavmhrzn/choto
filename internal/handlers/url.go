package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sulavmhrzn/choto/internal/dtos"
	"github.com/sulavmhrzn/choto/internal/service"
)

// Shorten godoc
// @Summary      Shorten a URL
// @Description  Creates a short link for a long URL with optional custom alias and expiration time.
// @Tags         urls
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body      dtos.ShortenRequest  true  "Shorten Request Body"
// @Success      201     {object}  dtos.ShortenResponse
// @Header       201     {string}  X-RateLimit-Limit      "Total requests allowed per window"
// @Header       201     {string}  X-RateLimit-Remaining  "Remaining requests in current window"
// @Failure      400     {object}  map[string]string "Invalid URL or Alias already taken"
// @Failure      401     {object}  map[string]string "Unauthorized"
// @Failure      500     {object}  map[string]string "Internal server error"
// @Router       /urls/shorten [post]
func (h *Handler) Shorten(c *gin.Context) {
	var req dtos.ShortenRequest
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
	c.JSON(http.StatusCreated, dtos.ShortenResponse{
		ID:        url.ID,
		ShortCode: url.ShortCode,
		LongURL:   url.LongURL,
		Clicks:    url.Clicks,
		ShortURL:  h.Config.BaseURL + "/" + url.ShortCode,
		ExpiresAt: url.ExpiresAt,
		CreatedAt: url.CreatedAt,
	})
}

// Redirect godoc
// @Summary      Redirect to long URL
// @Description  Retrieves the original URL associated with the short code, records click analytics (IP, UA, Referer), and performs a 302 redirect.
// @Tags         redirection
// @Param        code  path      string  true  "Short URL Code"
// @Success      302   {string}  string  "Redirecting to original URL"
// @Failure      400   {object}  map[string]string "Short code required"
// @Failure      404   {object}  map[string]string "Short link not found"
// @Failure      410   {object}  map[string]string "Short link has expired"
// @Failure      500   {object}  map[string]string "Internal server error"
// @Router       /{code} [get]
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
	h.Service.URLService.DeprecatedTrackClick(code)
	h.Service.URLService.RecordClick(c.Request.Context(), code, c.ClientIP(), c.Request.UserAgent(), c.Request.Referer())
	c.Redirect(http.StatusFound, longURL)
}

// GetStats godoc
// @Summary      Get analytics for a short code
// @Description  Returns detailed click statistics including browser, platform, and daily usage data.
// @Tags         urls
// @Produce      json
// @Security     BearerAuth
// @Param        code  path      string  true  "Short URL Code"
// @Success      200   {object}  dtos.URLStatsResponse
// @Failure      401   {object}  map[string]string "Unauthorized"
// @Failure      404   {object}  map[string]string "Short link not found"
// @Failure      410   {object}  map[string]string "Short link has expired"
// @Failure      500   {object}  map[string]string "Internal server error"
// @Router       /urls/stats/{code} [get]
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
	c.JSON(http.StatusOK, dtos.URLStatsResponse{
		LongURL:   stats.LongURL,
		ShortCode: stats.ShortCode,
		Clicks:    stats.Clicks,
		CreatedAt: stats.CreatedAt,
		ExpiresAt: stats.ExpiresAt,
	})
}

// DeleteURL godoc
// @Summary      Delete a short URL
// @Description  Permanently removes a short URL and its associated analytics. Only the owner can delete the link.
// @Tags         urls
// @Produce      json
// @Security     BearerAuth
// @Param        code  path  string  true  "Short URL Code"
// @Success      204   "No Content - URL successfully deleted"
// @Failure      401   {object}   map[string]string "Unauthorized"
// @Failure      404   {object}   map[string]string "URL not found or doesn't belong to user"
// @Failure      500   {object}   map[string]string "Internal server error"
// @Router       /urls/{code} [delete]
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

// GetURLClicks godoc
// @Summary      List raw click logs
// @Description  Returns a list of individual click events for a specific short code, including IP, User-Agent, and Referer.
// @Tags         clicks
// @Produce      json
// @Security     BearerAuth
// @Param        code   path      string  true  "Short URL Code"
// @Param        limit  query     int     false "Number of records to return (default 10)" default(10)
// @Success      200    {object}  []dtos.ClickResponse
// @Failure      400    {object}  map[string]string "Invalid limit parameter"
// @Failure      401    {object}  map[string]string "Unauthorized"
// @Failure      404    {object}  map[string]string "URL not found"
// @Router       /clicks/{code}/ [get]
func (h *Handler) GetURLClicks(c *gin.Context) {
	code := c.Param("code")
	userID := c.GetInt64("user_id")
	limit, err := strconv.ParseInt(c.DefaultQuery("limit", "10"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer type"})
		return
	}
	clicks, err := h.Service.URLService.ListURLClicks(c.Request.Context(), code, userID, limit)
	if err != nil {
		h.Logger.Error("failed to list url clicks", "code", code, "user_id", userID, "limit", limit, "err", err)
		if errors.Is(err, service.ErrURLNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	var clickResponse []*dtos.ClickResponse
	for _, click := range clicks {
		clickResponse = append(clickResponse, dtos.MapClickToDTO(click))
	}
	c.JSON(http.StatusOK, gin.H{
		"clicks": clickResponse,
	})
}

// GetURLClicksStat godoc
// @Summary      Get time-series click statistics
// @Description  Returns aggregated daily click counts for a specific short code. Ideal for rendering analytics charts.
// @Tags         clicks
// @Produce      json
// @Security     BearerAuth
// @Param        code  path      string  true  "Short URL Code"
// @Success      200   {object}  dtos.ClickStatsResponse
// @Failure      401   {object}  map[string]string "Unauthorized"
// @Failure      404   {object}  map[string]string "URL not found"
// @Failure      500   {object}  map[string]string "Internal server error"
// @Router       /clicks/{code}/stats [get]
func (h *Handler) GetURLClicksStat(c *gin.Context) {
	code := c.Param("code")
	userID := c.GetInt64("user_id")
	stats, err := h.Service.URLService.GetClickStats(c.Request.Context(), code, userID)
	if err != nil {
		h.Logger.Error("failed to list url clicks", "code", code, "user_id", userID, "err", err)
		if errors.Is(err, service.ErrURLNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusOK, dtos.ClickStatsResponse{
		ShortCode: code,
		Stats:     dtos.MapClickStatToDTO(*stats),
	})
}

// GenerateQRCode godoc
// @Summary      Generate QR Code
// @Description  Generates a PNG QR code for a given short code. Supports direct viewing or forced download.
// @Tags         urls
// @Produce      image/png
// @Param        code      path      string  true   "Short URL Code"
// @Param        download  query     bool    false  "Force download as attachment"
// @Success      200       {file}    binary  "QR Code image in PNG format"
// @Failure      404       {object}  map[string]string "URL not found"
// @Failure      500       {object}  map[string]string "Internal server error"
// @Router       /urls/{code}/qr [get]
func (h *Handler) GenerateQRCode(c *gin.Context) {
	code := c.Param("code")
	data, err := h.Service.URLService.GenerateQRCode(c.Request.Context(), code)
	if err != nil {
		if errors.Is(err, service.ErrURLNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	if c.Query("download") == "true" {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s_qr.png", code))
	}
	c.Data(http.StatusOK, "image/png", data)
}
