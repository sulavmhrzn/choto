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
