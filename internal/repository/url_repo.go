package repository

import (
	"context"
	"database/sql"
	"encoding/json"
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
	UserID    int64      `json:"user_id"`
	Clicks    int        `json:"clicks"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type URLStats struct {
	LongURL   string     `json:"long_url"`
	ShortCode string     `json:"short_code"`
	Clicks    int        `json:"clicks"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type Click struct {
	URLID       int64     `json:"url_id"`
	IpAddress   string    `json:"ip_address"`
	CountryCode string    `json:"country_code"`
	UserAgent   string    `json:"user_agent"`
	DeviceType  string    `json:"device_type"`
	Referrer    string    `json:"referrer"`
	IsBot       bool      `json:"is_bot"`
	ClickedAt   time.Time `json:"clicked_at"`
}

func (r *URLRepository) Create(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*URL, error) {
	var url URL
	var id uint64
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	if alias == "" {
		query := `INSERT INTO urls (long_url, expires_at, user_id) VALUES ($1, $2, $3) RETURNING id`
		args := []any{longURL, expiresAt, userID}
		err = tx.QueryRowContext(ctx, query, args...).Scan(&id)
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
		query := `INSERT INTO urls (long_url, expires_at, short_code, user_id) VALUES ($1, $2, $3, $4) 
		RETURNING
		id, long_url, short_code, expires_at, created_at, clicks`
		args := []any{longURL, expiresAt, alias, userID}
		err = tx.QueryRowContext(ctx, query, args...).Scan(
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

func (r *URLRepository) GetByCode(ctx context.Context, code string) (*URL, error) {
	val, err := r.rdb.Get(ctx, code).Result()
	if err == nil {
		var cachedURL URL
		if err := json.Unmarshal([]byte(val), &cachedURL); err == nil {
			return &cachedURL, nil
		}
	}

	var url URL
	query := `SELECT id, long_url, short_code, expires_at, user_id FROM urls WHERE short_code = $1`
	err = r.db.QueryRowContext(ctx, query, code).Scan(&url.ID, &url.LongURL, &url.ShortCode, &url.ExpiresAt, &url.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: code %s not found", ErrNoRows, code)
		}
		return nil, fmt.Errorf("database query failed: %w", err)
	}

	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		return nil, ErrLinkExpired
	}

	cacheTTL := 24 * time.Hour
	if url.ExpiresAt != nil {
		cacheTTL = time.Until(*url.ExpiresAt)
	}

	if cacheTTL > 0 {
		data, _ := json.Marshal(url)
		_ = r.rdb.Set(ctx, code, data, cacheTTL)
	}

	return &url, nil
}

func (r *URLRepository) IncrementClick(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `UPDATE urls SET clicks = clicks + 1 WHERE short_code = $1`
	_, err := r.db.ExecContext(ctx, query, code)
	return err
}

func (r *URLRepository) GetStats(ctx context.Context, code string, userID int64) (*URLStats, error) {
	query := `SELECT urls.long_url, urls.short_code, urls.clicks, urls.created_at, urls.expires_at
	FROM urls
	WHERE short_code = $1 AND urls.user_id = $2
	`

	var stats URLStats
	var expiresAt *time.Time
	err := r.db.QueryRowContext(ctx, query, code, userID).Scan(
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

func (r *URLRepository) DeleteByID(ctx context.Context, code string, userID int64) error {
	query := `DELETE FROM urls WHERE short_code = $1 AND user_id = $2`
	result, err := r.db.ExecContext(ctx, query, code, userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNoRows
	}
	return nil
}

func (r *URLRepository) RecordClicks(click Click) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query := `INSERT INTO clicks 
	(url_id, ip_address, country_code, user_agent, device_type, referrer, is_bot)
	VALUES 
	($1, $2, $3, $4, $5, $6, $7)`
	args := []any{click.URLID, click.IpAddress, click.CountryCode, click.UserAgent, click.DeviceType, click.Referrer, click.IsBot}
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *URLRepository) ListClicks(ctx context.Context, urlID, limit int64) ([]*Click, error) {
	query := `
	SELECT 
		url_id,
		clicked_at,
		ip_address,
		country_code,
		user_agent,
		device_type,
		is_bot,
		referrer
	FROM clicks
	WHERE url_id = $1 
	ORDER BY clicked_at DESC
	LIMIT $2
	`
	clicks := []*Click{}
	rows, err := r.db.QueryContext(ctx, query, urlID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var click Click
		err := rows.Scan(
			&click.URLID,
			&click.ClickedAt,
			&click.IpAddress,
			&click.CountryCode,
			&click.UserAgent,
			&click.DeviceType,
			&click.IsBot,
			&click.Referrer,
		)
		if err != nil {
			return nil, err
		}
		clicks = append(clicks, &click)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return clicks, nil
}
