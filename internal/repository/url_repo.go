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
	ErrNoRows = errors.New("no records found")
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

func (r *URLRepository) Create(ctx context.Context, longURL string) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var id uint64
	err = tx.QueryRowContext(ctx, "INSERT INTO urls (long_url) VALUES ($1) RETURNING id", longURL).Scan(&id)
	if err != nil {
		return "", err
	}

	shortCode, err := encoder.Encode(id, r.cfg.SecretKey)
	if err != nil {
		return "", err
	}

	_, err = tx.ExecContext(ctx, "UPDATE urls SET short_code = $1 WHERE id = $2", shortCode, id)
	if err != nil {
		return "", err
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}

	_ = r.rdb.Set(ctx, shortCode, longURL, 24*time.Hour)
	return shortCode, nil
}

func (r *URLRepository) GetByCode(ctx context.Context, code string) (string, error) {
	longURL, err := r.rdb.Get(ctx, code).Result()
	if err == nil {
		return longURL, nil
	}

	query := `SELECT long_url FROM urls WHERE short_code = $1`
	err = r.db.QueryRowContext(ctx, query, code).Scan(&longURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: code %s not found", ErrNoRows, code)
		}
		return "", fmt.Errorf("database query failed: %w", err)
	}

	_ = r.rdb.Set(ctx, code, longURL, 24*time.Hour)
	return longURL, nil
}
