package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/pkg/encoder"
)

var (
	ErrNoRows      = errors.New("no records found")
	ErrLinkExpired = errors.New("link has expired")
)

type URLRepository struct {
	db  *sql.DB
	rdb *redis.Client
	cfg *config.Config
}

func NewURLRepository(db *sql.DB, rdb *redis.Client, cfg *config.Config) *URLRepository {
	return &URLRepository{
		db:  db,
		rdb: rdb,
		cfg: cfg,
	}
}

type URL struct {
	ID        int64      `json:"id"`
	LongURL   string     `json:"long_url"`
	ShortCode string     `json:"short_code"`
	Clicks    int        `json:"clicks"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type URLStats struct {
	LongURL   string    `json:"long_url"`
	ShortCode string    `json:"short_code"`
	Clicks    int       `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

func (r *URLRepository) Create(ctx context.Context, longURL string, expiresAt *time.Time) (*URL, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id uint64
	err = tx.QueryRowContext(ctx, "INSERT INTO urls (long_url, expires_at) VALUES ($1, $2) RETURNING id", longURL, expiresAt).Scan(&id)
	if err != nil {
		return nil, err
	}

	shortCode, err := encoder.Encode(id, r.cfg.SecretKey)
	if err != nil {
		return nil, err
	}

	var url URL
	updateQuery := `UPDATE urls SET short_code = $1 WHERE id = $2
	 RETURNING id, long_url, short_code, clicks, created_at, expires_at
	`
	err = tx.QueryRowContext(ctx, updateQuery, shortCode, id).Scan(
		&url.ID,
		&url.LongURL,
		&url.ShortCode,
		&url.Clicks,
		&url.CreatedAt,
		&url.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	cacheTTL := 24 * time.Hour
	if url.ExpiresAt != nil {
		cacheTTL = time.Until(*url.ExpiresAt)
	}
	if cacheTTL > 0 {
		_ = r.rdb.Set(ctx, shortCode, longURL, cacheTTL)
	}
	return &url, nil
}

func (r *URLRepository) GetByCode(ctx context.Context, code string) (string, error) {
	longURL, err := r.rdb.Get(ctx, code).Result()
	if err == nil {
		return longURL, nil
	}

	var url URL
	query := `SELECT long_url, expires_at FROM urls WHERE short_code = $1`
	err = r.db.QueryRowContext(ctx, query, code).Scan(&url.LongURL, &url.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: code %s not found", ErrNoRows, code)
		}
		return "", fmt.Errorf("database query failed: %w", err)
	}

	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		return "", ErrLinkExpired
	}

	cacheTTL := 24 * time.Hour
	if url.ExpiresAt != nil {
		cacheTTL = time.Until(*url.ExpiresAt)
	}

	if cacheTTL > 0 {
		_ = r.rdb.Set(ctx, code, url.LongURL, cacheTTL)
	}

	_ = r.rdb.Set(ctx, code, url.LongURL, 24*time.Hour)
	return url.LongURL, nil
}

func (r *URLRepository) IncrementClick(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `UPDATE urls SET clicks = clicks + 1 WHERE short_code = $1`
	_, err := r.db.ExecContext(ctx, query, code)
	return err
}

func (r *URLRepository) GetStats(ctx context.Context, code string) (*URLStats, error) {
	query := `SELECT long_url, short_code, clicks, created_at FROM urls WHERE short_code = $1`

	var stats URLStats
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&stats.LongURL,
		&stats.ShortCode,
		&stats.Clicks,
		&stats.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, err
	}
	return &stats, nil
}
