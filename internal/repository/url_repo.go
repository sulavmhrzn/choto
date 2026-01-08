package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/pkg/encoder"
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

func (r *URLRepository) Create(ctx context.Context, longURL string, expiresAt *time.Time, alias string) (*URL, error) {
	var url URL
	var id uint64
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	if alias == "" {
		query := `INSERT INTO urls (long_url, expires_at) VALUES ($1, $2) RETURNING id`
		err = tx.QueryRowContext(ctx, query, longURL, expiresAt).Scan(&id)
		if err != nil {
			return nil, err
		}

		shortCode, err := encoder.Encode(id, r.cfg.SecretKey)
		if err != nil {
			return nil, err
		}

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
	} else {
		query := `INSERT INTO urls (long_url, expires_at, short_code) VALUES ($1, $2, $3) 
		RETURNING
		id, long_url, short_code, expires_at, created_at, clicks`
		err = tx.QueryRowContext(ctx, query, longURL, expiresAt, alias).Scan(
			&url.ID,
			&url.LongURL,
			&url.ShortCode,
			&url.ExpiresAt,
			&url.CreatedAt,
			&url.Clicks,
		)
		if err != nil {
			if err, ok := err.(*pq.Error); ok {
				if err.Code == "23505" {
					return nil, ErrUniqueShortCode
				}
			}
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	cacheTTL := 24 * time.Hour
	if url.ExpiresAt != nil {
		cacheTTL = time.Until(*url.ExpiresAt)
	}
	if cacheTTL > 0 {
		_ = r.rdb.Set(ctx, url.ShortCode, url.LongURL, cacheTTL)
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
	query := `SELECT long_url, short_code, clicks, created_at, expires_at FROM urls WHERE short_code = $1`

	var stats URLStats
	var expiresAt *time.Time
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&stats.LongURL,
		&stats.ShortCode,
		&stats.Clicks,
		&stats.CreatedAt,
		&expiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, err
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, ErrLinkExpired
	}

	return &stats, nil
}

func (r *URLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM urls WHERE expires_at IS NOT NULL AND expires_at < NOW()`
	res, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
