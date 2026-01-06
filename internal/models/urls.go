package models

import "time"

type URL struct {
	ID        uint64    `db:"id"`
	LongURL   string    `db:"long_url"`
	ShortCode string    `db:"short_code"`
	CreatedAt time.Time `db:"created_at"`
}
