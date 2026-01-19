package dtos

import "time"

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email" example:"user@example.com"`
	Password string `json:"password" binding:"required,min=8" example:"strongpassword123"`
}

type RegisterResponse struct {
	Email     string    `json:"email" example:"user@example.com"`
	CreatedAt time.Time `json:"created_at" example:"2026-01-14T14:21:25Z"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email" example:"user@example.com"`
	Password string `json:"password" binding:"required" example:"yourpassword123"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"def789..."`
	TokenType    string `json:"token_type" example:"Bearer"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required" example:"def789..."`
}

type RefreshResponse struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type UserResponse struct {
	ID        int64     `json:"id" example:"1"`
	Email     string    `json:"email" example:"user@example.com"`
	CreatedAt time.Time `json:"created_at" example:"2026-01-14T14:21:25Z"`
}

type DashboardResponse struct {
	TotalLinks  uint               `json:"total_links" example:"12"`
	TotalClicks uint               `json:"total_clicks" example:"450"`
	ActiveLinks uint               `json:"active_links" example:"10"`
	RecentLinks []URLStatsResponse `json:"recent_links"`
}
