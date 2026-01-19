package models

import "time"

type Dashboard struct {
	TotalLinks  uint
	TotalClicks uint
	ActiveLinks uint
	RecentLinks []URLStats
}

type User struct {
	ID           int64
	Email        string
	CreatedAt    time.Time
	PasswordHash string
}
