package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sulavmhrzn/choto/internal/service"
)

func (h *Handler) Register(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.UserService.CreateUser(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.Logger.Error("failed to create user", "err", err)
		if errors.Is(err, service.ErrEmailAlreadyInUse) {
			c.JSON(http.StatusConflict, gin.H{"error": "Email already in use"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
	}
	c.JSON(http.StatusCreated, gin.H{
		"email":      user.Email,
		"created_at": user.CreatedAt,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := h.UserService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.Logger.Error("failed to login user", "email", req.Email, "err", err)
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"token_type":    "Bearer",
	})
}

func (h *Handler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	newAccessToken, err := h.UserService.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.Logger.Error("failed to generate new access token", "err", err)
		if errors.Is(err, service.ErrRefreshTokenExpired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_token": newAccessToken})
}

func (h *Handler) GetMe(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		h.Logger.Error("user_id not found in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	user, err := h.UserService.GetUserByID(c.Request.Context(), userID.(int64))
	if err != nil {
		h.Logger.Error("failed to fetch user from database", "userId", userID, "err", err)
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": user.ID, "email": user.Email, "created_at": user.CreatedAt})
}
