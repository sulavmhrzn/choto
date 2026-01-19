package dtos

import (
	"time"

	"github.com/sulavmhrzn/choto/internal/models"
)

type ShortenRequest struct {
	URL       string `json:"url" binding:"required,url" example:"https://github.com/sulav/choto"`
	ExpiresIn int    `json:"expires_in_hours" binding:"omitempty,number,gt=0" example:"24"`
	Alias     string `json:"alias" binding:"omitempty,alphanum,min=3,max=10" example:"my-link"`
}

type ShortenResponse struct {
	ID        int64      `json:"id" example:"101"`
	ShortCode string     `json:"short_code" example:"my-link"`
	LongURL   string     `json:"long_url" example:"https://github.com/sulav/choto"`
	Clicks    int        `json:"clicks" example:"0"`
	ShortURL  string     `json:"short_url" example:"http://localhost:8080/my-link"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type ClickStatDTO struct {
	TotalClicks int64          `json:"total_clicks" example:"1000"`
	BotClicks   int64          `json:"bot_clicks" example:"12"`
	TopCountry  string         `json:"top_country" example:"Nepal"`
	TopDevice   string         `json:"top_device" example:"Mobile"`
	ByDevice    map[string]int `json:"by_device" example:"Mobile:750,Desktop:250"`
	ByCountry   map[string]int `json:"by_country" example:"NP:800,US:150,IN:50"`
}

type ClickStatsResponse struct {
	ShortCode string       `json:"short_code" example:"choto-api"`
	Stats     ClickStatDTO `json:"stats"`
}

func MapClickStatToDTO(m models.ClickStat) ClickStatDTO {
	return ClickStatDTO{
		TotalClicks: m.TotalClicks,
		BotClicks:   m.BotClicks,
		TopCountry:  m.TopCountry,
		TopDevice:   m.TopDevice,
		ByDevice:    m.ByDevice,
		ByCountry:   m.ByCountry,
	}
}

type ClickResponse struct {
	URLID       int64     `json:"url_id" example:"45"`
	IpAddress   string    `json:"ip_address" example:"103.10.24.1"`
	CountryCode string    `json:"country_code" example:"NP"`
	UserAgent   string    `json:"user_agent" example:"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)..."`
	DeviceType  string    `json:"device_type" example:"Desktop"`
	Referrer    string    `json:"referrer" example:"https://twitter.com/"`
	IsBot       bool      `json:"is_bot" example:"false"`
	ClickedAt   time.Time `json:"clicked_at" example:"2026-01-19T11:20:00Z"`
}

func MapClickToDTO(m *models.Click) *ClickResponse {
	return &ClickResponse{
		URLID:       m.URLID,
		IpAddress:   m.IpAddress,
		CountryCode: m.CountryCode,
		UserAgent:   m.UserAgent,
		DeviceType:  m.DeviceType,
		Referrer:    m.Referrer,
		IsBot:       m.IsBot,
		ClickedAt:   m.ClickedAt,
	}
}

type URLStatsResponse struct {
	LongURL   string     `json:"long_url" example:"https://github.com/sulavmhrzn/choto"`
	ShortCode string     `json:"short_code" example:"choto-git"`
	Clicks    int        `json:"clicks" example:"154"`
	CreatedAt time.Time  `json:"created_at" example:"2026-01-14T15:04:05Z"`
	ExpiresAt *time.Time `json:"expires_at" example:"2026-02-14T15:04:05Z"`
}

type URLResponse struct {
	ID        int64      `json:"id" example:"45"`
	LongURL   string     `json:"long_url" example:"https://medium.com/@go-tips"`
	ShortCode string     `json:"short_code" example:"go-tips"`
	UserID    int64      `json:"user_id" example:"1"`
	Clicks    int        `json:"clicks" example:"20"`
	CreatedAt time.Time  `json:"created_at" example:"2026-01-19T10:00:00Z"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" example:"2027-01-19T10:00:00Z"`
}
