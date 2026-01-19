package models

import "time"

type ClickStat struct {
	TotalClicks int64
	BotClicks   int64
	TopCountry  string
	TopDevice   string
	ByDevice    map[string]int
	ByCountry   map[string]int
}

type Click struct {
	URLID       int64
	IpAddress   string
	CountryCode string
	UserAgent   string
	DeviceType  string
	Referrer    string
	IsBot       bool
	ClickedAt   time.Time
}

type URLStats struct {
	LongURL   string
	ShortCode string
	Clicks    int
	CreatedAt time.Time
	ExpiresAt *time.Time
}

type URL struct {
	ID        int64
	LongURL   string
	ShortCode string
	UserID    int64
	Clicks    int
	CreatedAt time.Time
	ExpiresAt *time.Time
}
