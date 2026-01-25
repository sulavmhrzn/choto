package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/models"
	"github.com/sulavmhrzn/choto/internal/pkg/encoder"
)

type URLRepository struct {
	db     *sql.DB
	rdb    *redis.Client
	cfg    *config.Config
	logger *slog.Logger
}

func NewURLRepository(db *sql.DB, rdb *redis.Client, cfg *config.Config, logger *slog.Logger) *URLRepository {
	return &URLRepository{
		db:     db,
		rdb:    rdb,
		cfg:    cfg,
		logger: logger,
	}
}

func (r *URLRepository) Create(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*models.URL, error) {
	var url models.URL
	var id uint64

	logger := r.logger.With(
		slog.Int64("user_id", userID),
		slog.String("long_url", longURL),
	)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		logger.ErrorContext(ctx, "failed to begin transaction", slog.Any("error", err))
		return nil, err
	}
	defer tx.Rollback()

	if alias == "" {
		query := `INSERT INTO urls (long_url, expires_at, user_id) VALUES ($1, $2, $3) RETURNING id`
		err = tx.QueryRowContext(ctx, query, longURL, expiresAt, userID).Scan(&id)
		if err != nil {
			logger.ErrorContext(ctx, "failed to insert url for auto-id", slog.Any("error", err))
			return nil, err
		}

		shortCode, err := encoder.Encode(id, r.cfg.SecretKey)
		if err != nil {
			logger.ErrorContext(ctx, "failed to encode short code", slog.Uint64("id", id), slog.Any("error", err))
			return nil, err
		}

		updateQuery := `UPDATE urls SET short_code = $1 WHERE id = $2
        RETURNING id, long_url, short_code, clicks, created_at, expires_at`

		err = tx.QueryRowContext(ctx, updateQuery, shortCode, id).Scan(
			&url.ID, &url.LongURL, &url.ShortCode, &url.Clicks, &url.CreatedAt, &url.ExpiresAt,
		)
		if err != nil {
			logger.ErrorContext(ctx, "failed to update short_code", slog.String("short_code", shortCode), slog.Any("error", err))
			return nil, err
		}
	} else {
		query := `INSERT INTO urls (long_url, expires_at, short_code, user_id) VALUES ($1, $2, $3, $4) 
        RETURNING id, long_url, short_code, expires_at, created_at, clicks`

		err = tx.QueryRowContext(ctx, query, longURL, expiresAt, alias, userID).Scan(
			&url.ID, &url.LongURL, &url.ShortCode, &url.ExpiresAt, &url.CreatedAt, &url.Clicks,
		)
		if err != nil {
			if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
				logger.WarnContext(ctx, "alias already exists", slog.String("alias", alias))
				return nil, ErrUniqueShortCode
			}
			logger.ErrorContext(ctx, "failed to insert custom alias", slog.String("alias", alias), slog.Any("error", err))
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		logger.ErrorContext(ctx, "failed to commit transaction", slog.Any("error", err))
		return nil, err
	}

	cacheTTL := 24 * time.Hour
	if url.ExpiresAt != nil {
		cacheTTL = time.Until(*url.ExpiresAt)
	}

	if cacheTTL > 0 {
		err := r.rdb.Set(ctx, url.ShortCode, url.LongURL, cacheTTL).Err()
		if err != nil {
			logger.WarnContext(ctx, "cache update failed after db commit",
				slog.String("short_code", url.ShortCode),
				slog.Any("error", err),
			)
		} else {
			logger.DebugContext(ctx, "cache populated",
				slog.String("short_code", url.ShortCode),
				slog.Duration("ttl", cacheTTL),
			)
		}
	}

	logger.InfoContext(ctx, "url created successfully", slog.String("short_code", url.ShortCode))
	return &url, nil
}

func (r *URLRepository) GetByCode(ctx context.Context, code string) (*models.URL, error) {
	val, err := r.rdb.Get(ctx, code).Result()
	if err == nil {
		var cachedURL models.URL
		if err := json.Unmarshal([]byte(val), &cachedURL); err == nil {
			r.logger.DebugContext(ctx, "cache hit", slog.String("short_code", code))
			return &cachedURL, nil
		}
		r.logger.WarnContext(ctx, "failed to unmarshal cached url",
			slog.String("short_code", code),
			slog.Any("error", err),
		)
	}
	r.logger.DebugContext(ctx, "cache miss", slog.String("short_code", code))

	var url models.URL
	query := `SELECT id, long_url, short_code, expires_at, user_id FROM urls WHERE short_code = $1`
	err = r.db.QueryRowContext(ctx, query, code).Scan(
		&url.ID, &url.LongURL, &url.ShortCode, &url.ExpiresAt, &url.UserID,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.InfoContext(ctx, "url not found in database", slog.String("short_code", code))
			return nil, fmt.Errorf("%w: code %s not found", ErrNoRows, code)
		}
		r.logger.ErrorContext(ctx, "database query failed",
			slog.String("short_code", code),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("database query failed: %w", err)
	}

	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		r.logger.WarnContext(ctx, "url expired",
			slog.String("short_code", code),
			slog.Time("expires_at", *url.ExpiresAt),
		)
		return nil, ErrLinkExpired
	}

	cacheTTL := 24 * time.Hour
	if url.ExpiresAt != nil {
		cacheTTL = time.Until(*url.ExpiresAt)
	}

	if cacheTTL > 0 {
		data, _ := json.Marshal(url)
		err := r.rdb.Set(ctx, code, data, cacheTTL).Err()
		if err != nil {
			r.logger.WarnContext(ctx, "failed to backfill cache",
				slog.String("short_code", code),
				slog.Any("error", err),
			)
		} else {
			r.logger.DebugContext(ctx, "cache backfilled",
				slog.String("short_code", code),
				slog.Duration("ttl", cacheTTL),
			)
		}
	}

	return &url, nil
}

func (r *URLRepository) IncrementClick(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `UPDATE urls SET clicks = clicks + 1 WHERE short_code = $1`

	result, err := r.db.ExecContext(ctx, query, code)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to increment click count",
			slog.String("short_code", code),
			slog.Any("error", err),
		)
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		r.logger.WarnContext(ctx, "click increment attempted on non-existent code",
			slog.String("short_code", code),
		)
	} else {
		r.logger.DebugContext(ctx, "click count incremented",
			slog.String("short_code", code),
		)
	}

	return nil
}

func (r *URLRepository) GetStats(ctx context.Context, code string, userID int64) (*models.URLStats, error) {
	logger := r.logger.With(
		slog.String("short_code", code),
		slog.Int64("user_id", userID),
	)

	query := `SELECT urls.long_url, urls.short_code, urls.clicks, urls.created_at, urls.expires_at
    FROM urls
    WHERE short_code = $1 AND urls.user_id = $2`

	var stats models.URLStats
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
			logger.DebugContext(ctx, "stats query returned no rows")
			return nil, ErrNoRows
		}

		logger.ErrorContext(ctx, "failed to query link stats", slog.Any("error", err))
		return nil, err
	}

	if expiresAt != nil && expiresAt.Before(time.Now()) {
		logger.WarnContext(ctx, "attempted to view stats for expired link",
			slog.Time("expires_at", *expiresAt),
		)
		return nil, ErrLinkExpired
	}

	logger.DebugContext(ctx, "stats retrieved successfully", slog.Int("clicks", stats.Clicks))

	return &stats, nil
}

func (r *URLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM urls WHERE expires_at IS NOT NULL AND expires_at < NOW()`

	r.logger.InfoContext(ctx, "starting cleanup of expired urls")

	res, err := r.db.ExecContext(ctx, query)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to delete expired urls",
			slog.Any("error", err),
		)
		return 0, err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to retrieve rows affected count",
			slog.Any("error", err),
		)
		return 0, err
	}

	if rowsAffected > 0 {
		r.logger.InfoContext(ctx, "expired urls cleaned up",
			slog.Int64("deleted_count", rowsAffected),
		)
	} else {
		r.logger.DebugContext(ctx, "no expired urls found to delete")
	}

	return rowsAffected, nil
}

func (r *URLRepository) DeleteByID(ctx context.Context, code string, userID int64) error {
	logger := r.logger.With(
		slog.String("short_code", code),
		slog.Int64("user_id", userID),
	)

	query := `DELETE FROM urls WHERE short_code = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, code, userID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to execute delete query", slog.Any("error", err))
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		logger.ErrorContext(ctx, "failed to get rows affected for delete", slog.Any("error", err))
		return err
	}

	if count == 0 {
		logger.WarnContext(ctx, "delete attempted on non-existent or unauthorized link")
		return ErrNoRows
	}

	err = r.rdb.Del(ctx, code).Err()
	if err != nil {
		logger.WarnContext(ctx, "failed to remove deleted url from cache", slog.Any("error", err))
	} else {
		logger.DebugContext(ctx, "cache invalidated for deleted url")
	}

	logger.InfoContext(ctx, "url deleted successfully")
	return nil
}

func (r *URLRepository) RecordClicks(click models.Click) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger := r.logger.With(
		slog.Int64("url_id", click.URLID),
		slog.String("ip", click.IpAddress),
		slog.Bool("is_bot", click.IsBot),
	)

	query := `INSERT INTO clicks 
    (url_id, ip_address, country_code, user_agent, device_type, referrer, is_bot)
    VALUES 
    ($1, $2, $3, $4, $5, $6, $7)`

	args := []any{
		click.URLID,
		click.IpAddress,
		click.CountryCode,
		click.UserAgent,
		click.DeviceType,
		click.Referrer,
		click.IsBot,
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		logger.ErrorContext(ctx, "failed to record click analytics",
			slog.Any("error", err),
		)
		return err
	}

	logger.DebugContext(ctx, "click analytics recorded",
		slog.String("country", click.CountryCode),
		slog.String("device", click.DeviceType),
	)

	return nil
}

func (r *URLRepository) ListClicks(ctx context.Context, urlID, limit int64) ([]*models.Click, error) {
	logger := r.logger.With(
		slog.Int64("url_id", urlID),
		slog.Int64("limit", limit),
	)

	query := `
    SELECT 
        url_id, clicked_at, ip_address, country_code, user_agent, device_type, is_bot, referrer
    FROM clicks
    WHERE url_id = $1 
    ORDER BY clicked_at DESC
    LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, urlID, limit)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query clicks list", slog.Any("error", err))
		return nil, err
	}
	defer rows.Close()

	clicks := []*models.Click{}
	for rows.Next() {
		var click models.Click
		err := rows.Scan(
			&click.URLID, &click.ClickedAt, &click.IpAddress, &click.CountryCode,
			&click.UserAgent, &click.DeviceType, &click.IsBot, &click.Referrer,
		)
		if err != nil {
			logger.ErrorContext(ctx, "failed to scan click row", slog.Any("error", err))
			return nil, err
		}
		clicks = append(clicks, &click)
	}

	if err := rows.Err(); err != nil {
		logger.ErrorContext(ctx, "error during clicks row iteration", slog.Any("error", err))
		return nil, err
	}

	logger.DebugContext(ctx, "successfully retrieved clicks list", slog.Int("count", len(clicks)))
	return clicks, nil
}

func (r *URLRepository) GetClickStats(ctx context.Context, urlID int64) (*models.ClickStat, error) {
	logger := r.logger.With(slog.Int64("url_id", urlID))

	query := `
    WITH country_stats AS (
        SELECT country_code AS key, COUNT(*) AS value
        FROM clicks WHERE url_id = $1
        GROUP BY country_code
    ),
    device_stats AS (
        SELECT device_type AS key, COUNT(*) AS value
        FROM clicks WHERE url_id = $1
        GROUP BY device_type
    )
    SELECT
        (SELECT COUNT(*) FROM clicks WHERE url_id = $1) as total_clicks,
        (SELECT COUNT(*) FILTER (WHERE is_bot=true) FROM clicks WHERE url_id = $1) AS bot_clicks, 
        (SELECT country_code FROM clicks WHERE url_id = $1 GROUP BY country_code ORDER BY COUNT(*) DESC LIMIT 1) as top_country,
        (SELECT device_type FROM clicks WHERE url_id = $1 GROUP BY device_type ORDER BY COUNT(*) DESC LIMIT 1) as top_device,
        (SELECT jsonb_object_agg(key, value) FROM country_stats) as by_country,
        (SELECT jsonb_object_agg(key, value) FROM device_stats) as by_device`

	var stats models.ClickStat
	var topCountry, topDevice sql.NullString
	var countryData, deviceData []byte

	err := r.db.QueryRowContext(ctx, query, urlID).Scan(
		&stats.TotalClicks,
		&stats.BotClicks,
		&topCountry,
		&topDevice,
		&countryData,
		&deviceData,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.DebugContext(ctx, "no stats found for url")
			return nil, nil
		}
		logger.ErrorContext(ctx, "failed to fetch click stats aggregation", slog.Any("error", err))
		return nil, err
	}

	stats.TopCountry = topCountry.String
	stats.TopDevice = topDevice.String

	if err := json.Unmarshal(countryData, &stats.ByCountry); err != nil {
		logger.WarnContext(ctx, "failed to unmarshal country stats", slog.Any("error", err))
	}
	if err := json.Unmarshal(deviceData, &stats.ByDevice); err != nil {
		logger.WarnContext(ctx, "failed to unmarshal device stats", slog.Any("error", err))
	}

	logger.DebugContext(ctx, "click stats aggregated successfully",
		slog.Int64("total", stats.TotalClicks),
	)
	return &stats, nil
}

func (r *URLRepository) RecordClicksBatch(ctx context.Context, clicks []models.Click) error {
	if len(clicks) == 0 {
		return nil
	}

	query := "INSERT INTO clicks (url_id, ip_address, user_agent, country_code, device_type, clicked_at, referrer) VALUES "

	values := []any{}
	placeholders := []string{}

	for i, click := range clicks {
		offset := i * 7
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1,
			offset+2,
			offset+3,
			offset+4,
			offset+5,
			offset+6,
			offset+7,
		))
		values = append(values,
			click.URLID,
			click.IpAddress,
			click.UserAgent,
			click.CountryCode,
			click.DeviceType,
			click.ClickedAt,
			click.Referrer,
		)
	}

	query += strings.Join(placeholders, ",")

	_, err := r.db.ExecContext(ctx, query, values...)
	return err
}
