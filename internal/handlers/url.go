package handlers

import (
	"errors"
	"fmt"
	"log/slog"
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
		h.Logger.WarnContext(c, "invalid shorten request body", slog.Any("error", err))
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
		switch {
		case errors.Is(err, service.ErrInvalidScheme):
			h.Logger.WarnContext(c, "shorten failed: invalid scheme", slog.String("url", req.URL))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Accepts only http/https schemes"})
		case errors.Is(err, service.ErrAliasAlreadyTaken):
			h.Logger.WarnContext(c, "shorten failed: alias taken", slog.String("alias", req.Alias))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Alias already taken"})
		case errors.Is(err, service.ErrReservedAlias):
			h.Logger.WarnContext(c, "shorten failed: reserved alias", slog.String("alias", req.Alias))
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%q is not allowed as an alias", req.Alias)})
		default:
			h.Logger.ErrorContext(c, "failed to shorten URL",
				slog.Int64("user_id", userID),
				slog.String("url", req.URL),
				slog.Any("error", err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}

	if val, exists := c.Get("rate_limit_count"); exists {
		if count, ok := val.(int64); ok {
			c.Header("X-RateLimit-Limit", strconv.FormatInt(int64(h.Config.RateLimitCount), 10))
			remaining := int64(h.Config.RateLimitCount) - count
			c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
		}
	}

	h.Logger.InfoContext(c, "url shortened",
		slog.Int64("user_id", userID),
		slog.String("short_code", url.ShortCode),
	)

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

	longURL, err := h.Service.URLService.GetOriginalURL(c, code)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrShortCodeRequired):
			h.Logger.WarnContext(c, "redirect failed: missing code")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Short code required"})
		case errors.Is(err, service.ErrURLNotFound):
			h.Logger.InfoContext(c, "redirect failed: code not found", slog.String("code", code))
			c.JSON(http.StatusNotFound, gin.H{"error": "Short link not found"})
		case errors.Is(err, service.ErrShortCodeExpired):
			h.Logger.WarnContext(c, "redirect failed: expired link", slog.String("code", code))
			c.JSON(http.StatusGone, gin.H{"error": "Short link has expired"})
		default:
			h.Logger.ErrorContext(c, "redirect failed: internal error",
				slog.String("code", code),
				slog.Any("error", err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}

	h.Service.URLService.DeprecatedTrackClick(code)
	h.Service.URLService.RecordClick(c, code, c.ClientIP(), c.Request.UserAgent(), c.Request.Referer())

	h.Logger.DebugContext(c, "redirecting user",
		slog.String("code", code),
		slog.String("target", longURL),
	)

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
		switch {
		case errors.Is(err, service.ErrURLNotFound):
			h.Logger.InfoContext(c, "stats lookup failed: not found or unauthorized",
				slog.String("code", code),
				slog.Int64("user_id", userID),
			)
			c.JSON(http.StatusNotFound, gin.H{"error": "Short link not found"})

		case errors.Is(err, service.ErrShortCodeExpired):
			h.Logger.WarnContext(c, "stats lookup failed: expired",
				slog.String("code", code),
			)
			c.JSON(http.StatusGone, gin.H{"error": "Short link has expired"})

		default:
			h.Logger.ErrorContext(c, "stats lookup failed: internal error",
				slog.String("code", code),
				slog.Any("error", err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}

	h.Logger.DebugContext(c, "stats retrieved successfully",
		slog.String("code", code),
		slog.Int64("user_id", userID),
	)

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
		if errors.Is(err, service.ErrURLNotFound) {
			h.Logger.WarnContext(c, "delete failed: not found or unauthorized",
				slog.String("code", code),
				slog.Int64("user_id", userID),
			)
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}

		h.Logger.ErrorContext(c, "delete failed: system error",
			slog.String("code", code),
			slog.Any("error", err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	h.Logger.InfoContext(c, "url deleted successfully",
		slog.String("code", code),
		slog.Int64("user_id", userID),
	)

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
		h.Logger.WarnContext(c, "invalid limit parameter",
			slog.String("limit_raw", c.Query("limit")),
			slog.Any("error", err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer type"})
		return
	}

	if limit > 100 {
		h.Logger.InfoContext(c, "capping high limit request", slog.Int64("original_limit", limit))
		limit = 100
	}

	clicks, err := h.Service.URLService.ListURLClicks(c.Request.Context(), code, userID, limit)
	if err != nil {
		if errors.Is(err, service.ErrURLNotFound) {
			h.Logger.InfoContext(c, "clicks list failed: not found or unauthorized",
				slog.String("code", code),
				slog.Int64("user_id", userID),
			)
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}

		h.Logger.ErrorContext(c, "failed to list url clicks",
			slog.String("code", code),
			slog.Int64("user_id", userID),
			slog.Any("error", err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	h.Logger.DebugContext(c, "successfully retrieved clicks list",
		slog.String("code", code),
		slog.Int("count", len(clicks)),
	)

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
		if errors.Is(err, service.ErrURLNotFound) {
			h.Logger.InfoContext(c, "click stats failed: unauthorized or not found",
				slog.String("code", code),
				slog.Int64("user_id", userID),
			)
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}

		h.Logger.ErrorContext(c, "failed to get click stats",
			slog.String("code", code),
			slog.Any("error", err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	h.Logger.DebugContext(c, "click stats retrieved successfully",
		slog.String("code", code),
		slog.Int64("total_clicks", stats.TotalClicks),
	)

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
			h.Logger.InfoContext(c, "qr generation failed: code not found",
				slog.String("code", code),
			)
			c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
			return
		}

		h.Logger.ErrorContext(c, "failed to generate qr code",
			slog.String("code", code),
			slog.Any("error", err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if c.Query("download") == "true" {
		h.Logger.DebugContext(c, "user initiated qr code download", slog.String("code", code))
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s_qr.png", code))
	}

	h.Logger.DebugContext(c, "serving qr code image",
		slog.String("code", code),
		slog.Int("size_bytes", len(data)),
	)

	c.Data(http.StatusOK, "image/png", data)
}
