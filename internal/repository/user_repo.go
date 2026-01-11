package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type UserRepository struct {
	DB *sql.DB
}

type Dashboard struct {
	TotalLinks  uint
	TotalClicks uint
	ActiveLinks uint
	RecentLinks []URLStats
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{
		DB: db,
	}
}

type User struct {
	ID           int64
	Email        string
	CreatedAt    time.Time
	PasswordHash string
}

func (r *UserRepository) Create(ctx context.Context, email string, passwordHash string) (*User, error) {
	query := `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, created_at`
	var user User
	err := r.DB.QueryRowContext(ctx, query, email, passwordHash).Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		if err, ok := err.(*pq.Error); ok {
			if err.Code == "23505" {
				return nil, ErrDuplicateEmail
			}
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT id, email, password_hash, created_at FROM users WHERE email = $1`
	var user User
	err := r.DB.QueryRowContext(ctx, query, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*User, error) {
	query := `SELECT id, email, password_hash, created_at FROM users WHERE id = $1`
	var user User
	err := r.DB.QueryRowContext(ctx, query, id).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetDashboard(ctx context.Context, userID int64) (*Dashboard, error) {
	summaryQuery := `
	SELECT 
	COUNT(*) AS total_links,
	COALESCE(SUM(clicks), 0) AS total_clicks,
	COUNT(*) FILTER (WHERE expires_at > NOW() OR expires_at IS NULL)
	FROM urls
	WHERE user_id = $1`
	recentLinksQuery := `
	SELECT short_code, long_url, clicks, created_at, expires_at
	FROM urls
	WHERE user_id = $1 ORDER BY created_at DESC LIMIT 10`

	var dashboard Dashboard
	err := r.DB.QueryRowContext(ctx, summaryQuery, userID).Scan(&dashboard.TotalLinks, &dashboard.TotalClicks, &dashboard.ActiveLinks)
	if err != nil {
		return nil, fmt.Errorf("summary query failed: %w", err)
	}
	rows, err := r.DB.QueryContext(ctx, recentLinksQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("recent links query failed: %w", err)
	}
	defer rows.Close()

	dashboard.RecentLinks = []URLStats{}
	for rows.Next() {
		var link URLStats

		err := rows.Scan(&link.ShortCode, &link.LongURL, &link.Clicks, &link.CreatedAt, &link.ExpiresAt)
		if err != nil {
			return nil, err
		}
		dashboard.RecentLinks = append(dashboard.RecentLinks, link)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &dashboard, nil
}
