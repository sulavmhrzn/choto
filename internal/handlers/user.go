package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sulavmhrzn/choto/internal/dtos"
	"github.com/sulavmhrzn/choto/internal/service"
)

// Register godoc
// @Summary      Register a new user
// @Description  Create a new user account with email and password.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      dtos.RegisterRequest  true  "Registration Details"
// @Success      201      {object}  dtos.RegisterResponse
// @Failure      400      {object}  map[string]string "Invalid input (e.g. invalid email or short password)"
// @Failure      409      {object}  map[string]string "Email already in use"
// @Failure      500      {object}  map[string]string "Internal server error"
// @Router       /auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req dtos.RegisterRequest

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.Service.UserService.CreateUser(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.Logger.Error("failed to create user", "err", err)
		if errors.Is(err, service.ErrEmailAlreadyInUse) {
			c.JSON(http.StatusConflict, gin.H{"error": "Email already in use"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusCreated, dtos.RegisterResponse{
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	})
}

// Login godoc
// @Summary      User Login
// @Description  Authenticate user and return JWT access and refresh tokens.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      dtos.LoginRequest  true  "Login Credentials"
// @Success      200      {object}  dtos.LoginResponse
// @Failure      400      {object}  map[string]string "Invalid request body"
// @Failure      401      {object}  map[string]string "Invalid credentials"
// @Failure      500      {object}  map[string]string "Internal server error"
// @Router       /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req dtos.LoginRequest

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := h.Service.UserService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.Logger.Error("failed to login user", "email", req.Email, "err", err)
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusOK, dtos.LoginResponse{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    "Bearer",
	})
}

// Refresh godoc
// @Summary      Refresh Access Token
// @Description  Exchanges a valid refresh token for a new short-lived access token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      dtos.RefreshRequest  true  "Refresh Token"
// @Success      200      {object}  dtos.RefreshResponse
// @Failure      400      {object}  map[string]string "Invalid request body"
// @Failure      401      {object}  map[string]string "Refresh token expired"
// @Failure      500      {object}  map[string]string "Internal server error"
// @Router       /auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req dtos.RefreshRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	newAccessToken, err := h.Service.UserService.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.Logger.Error("failed to generate new access token", "err", err)
		if errors.Is(err, service.ErrRefreshTokenExpired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusOK, dtos.RefreshResponse{AccessToken: newAccessToken})
}

// GetMe godoc
// @Summary      Get current user profile
// @Description  Returns the profile information of the currently authenticated user based on the JWT token.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  dtos.UserResponse
// @Failure      401  {object}  map[string]string "Unauthorized - Missing or invalid token"
// @Failure      404  {object}  map[string]string "User not found"
// @Failure      500  {object}  map[string]string "Internal server error"
// @Router       /auth/me [get]
func (h *Handler) GetMe(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		h.Logger.Error("user_id not found in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	user, err := h.Service.UserService.GetUserByID(c.Request.Context(), userID.(int64))
	if err != nil {
		h.Logger.Error("failed to fetch user from database", "userId", userID, "err", err)
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, dtos.UserResponse{ID: user.ID, Email: user.Email, CreatedAt: user.CreatedAt})
}

// GetDashboard godoc
// @Summary      Get user dashboard data
// @Description  Returns a summary of total links, clicks, and a list of the most recent links created by the user.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  dtos.DashboardResponse
// @Failure      401  {object}  map[string]string "Unauthorized"
// @Failure      500  {object}  map[string]string "Internal server error"
// @Router       /auth/dashboard [get]
func (h *Handler) GetDashboard(c *gin.Context) {
	userID := c.GetInt64("user_id")
	dashboard, err := h.Service.UserService.GetUserDashboard(c.Request.Context(), userID)
	if err != nil {
		h.Logger.Error("failed to get dashboard data", "user_id", userID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"summary": map[string]any{
			"total_links":  dashboard.TotalLinks,
			"total_clicks": dashboard.TotalClicks,
			"active_links": dashboard.ActiveLinks,
		},
		"recent_links": dashboard.RecentLinks,
	})
}
