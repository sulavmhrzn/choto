package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/mileusna/useragent"
	"github.com/oschwald/geoip2-golang/v2"
	"github.com/redis/go-redis/v9"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/sulavmhrzn/choto/internal/config"
	"github.com/sulavmhrzn/choto/internal/models"
	"github.com/sulavmhrzn/choto/internal/repository"
)

var (
	ErrInvalidScheme     = errors.New("the provided url contains an invalid scheme")
	ErrInvalidURL        = errors.New("the provided url is not valid")
	ErrURLNotFound       = errors.New("url not found")
	ErrShortCodeRequired = errors.New("short code is required")
	ErrShortCodeExpired  = errors.New("short code has already expired")
	ErrAliasAlreadyTaken = errors.New("alias is already taken")
	ErrReservedAlias     = errors.New("reserved alias is not permitted")
)

var ReservedAliases = map[string]struct{}{
	"health": {},
	"api":    {},
	"stats":  {},
	"ping":   {},
	"admin":  {},
	"static": {},
}
var ValidSchemes = map[string]struct{}{
	"http":  {},
	"https": {},
}

type Shortener interface {
	Shorten(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*models.URL, error)
	GetOriginalURL(ctx context.Context, code string) (string, error)
	GetStats(ctx context.Context, code string, userID int64) (*models.URLStats, error)
	DeprecatedTrackClick(code string)
	StartCleanupWorker(ctx context.Context, interval time.Duration)
	DeleteURL(ctx context.Context, code string, userID int64) error
	RecordClick(ctx context.Context, short_code, ip_address, user_agent, referrer string) error
	ListURLClicks(ctx context.Context, code string, userID, limit int64) ([]*models.Click, error)
	GetClickStats(ctx context.Context, code string, userID int64) (*models.ClickStat, error)
	GenerateQRCode(ctx context.Context, code string) ([]byte, error)
}

type URLRepository interface {
	Create(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*models.URL, error)
	GetByCode(ctx context.Context, code string) (*models.URL, error)
	IncrementClick(code string) error
	GetStats(ctx context.Context, code string, userID int64) (*models.URLStats, error)
	DeleteExpired(ctx context.Context) (int64, error)
	DeleteByID(ctx context.Context, code string, userID int64) error
	RecordClicks(click models.Click) error
	ListClicks(ctx context.Context, urlID, limit int64) ([]*models.Click, error)
	GetClickStats(ctx context.Context, urlID int64) (*models.ClickStat, error)
}

type URLService struct {
	repo   URLRepository
	rdb    *redis.Client
	logger *slog.Logger
	config *config.Config
}

func NewURLService(repo URLRepository, rdb *redis.Client, logger *slog.Logger, config *config.Config) Shortener {
	return &URLService{
		repo:   repo,
		rdb:    rdb,
		logger: logger,
		config: config,
	}
}

func (s *URLService) IsReserved(alias string) bool {
	_, exists := ReservedAliases[strings.ToLower(alias)]
	return exists
}

func (s *URLService) IsValidScheme(scheme string) bool {
	_, exists := ValidSchemes[scheme]
	return exists
}

func (s *URLService) parseUserAgent(uaString string) (string, bool) {
	ua := useragent.Parse(uaString)

	s.logger.Debug("parsed user agent",
		slog.String("device", ua.Device),
		slog.Bool("is_bot", ua.Bot),
	)

	if ua.Bot {
		return "bot", true
	}
	if ua.Mobile {
		return "mobile", false
	} else if ua.Tablet {
		return "tablet", false
	}
	return "desktop", false
}

func (s *URLService) getCountryCode(ipAddr string) string {
	db, _ := geoip2.Open("GeoLite2-Country.mmdb")
	defer db.Close()

	ip, err := netip.ParseAddr(ipAddr)
	if err != nil {
		s.logger.Warn("malformed ip address",
			slog.String("ip", ipAddr),
			slog.Any("error", err),
		)
		return "XX"
	}
	record, err := db.Country(ip)
	if err != nil {
		s.logger.Warn("ip country lookup failed",
			slog.String("ip", ipAddr),
			slog.Any("error", err),
		)
		return "XX"
	}
	if !record.HasData() {
		s.logger.Debug("ip has no country data", slog.String("ip", ipAddr))
		return "XX"
	}
	return record.RegisteredCountry.ISOCode
}

func (s *URLService) Shorten(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*models.URL, error) {
	logger := s.logger.With(
		slog.Int64("user_id", userID),
		slog.String("alias", alias),
	)

	cleanURL := strings.TrimSpace(longURL)
	u, err := url.ParseRequestURI(cleanURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		logger.WarnContext(ctx, "invalid url provided", slog.String("url", longURL))
		return nil, ErrInvalidURL
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	finalURL := u.String()

	if !s.IsValidScheme(u.Scheme) {
		logger.WarnContext(ctx, "unsupported url scheme", slog.String("scheme", u.Scheme))
		return nil, ErrInvalidScheme
	}

	if alias != "" {
		alias = strings.TrimSpace(strings.ToLower(alias))
		if s.IsReserved(alias) {
			logger.WarnContext(ctx, "attempted to use reserved alias", slog.String("alias", alias))
			return nil, ErrReservedAlias
		}
	}

	createdURL, err := s.repo.Create(ctx, finalURL, expiresAt, alias, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUniqueShortCode) {
			logger.WarnContext(ctx, "alias collision", slog.String("alias", alias))
			return nil, ErrAliasAlreadyTaken
		}

		logger.ErrorContext(ctx, "repository failed to create short url",
			slog.String("final_url", finalURL),
			slog.Any("error", err),
		)
		return nil, err
	}

	logger.InfoContext(ctx, "url shortened successfully",
		slog.String("short_code", createdURL.ShortCode),
	)

	return createdURL, nil
}

func (s *URLService) GetOriginalURL(ctx context.Context, code string) (string, error) {
	logger := s.logger.With(slog.String("short_code", code))

	if code == "" {
		logger.WarnContext(ctx, "lookup failed: empty short code provided")
		return "", ErrShortCodeRequired
	}

	url, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			logger.InfoContext(ctx, "lookup failed: code not found")
			return "", ErrURLNotFound
		}

		if errors.Is(err, repository.ErrLinkExpired) {
			logger.WarnContext(ctx, "lookup failed: link is expired")
			return "", ErrShortCodeExpired
		}

		logger.ErrorContext(ctx, "repository lookup failed", slog.Any("error", err))
		return "", fmt.Errorf("lookup failed for code %s: %w", code, err)
	}

	if url.LongURL == "" {
		logger.ErrorContext(ctx, "data integrity error: long_url is empty in database")
		return "", ErrURLNotFound
	}

	logger.DebugContext(ctx, "url resolved successfully", slog.String("long_url", url.LongURL))

	return url.LongURL, nil
}

func (s *URLService) DeprecatedTrackClick(code string) {
	ctx := context.Background()

	go func() {
		s.logger.DebugContext(ctx, "executing deprecated click tracking",
			slog.String("short_code", code),
		)

		err := s.repo.IncrementClick(code)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to increment click in deprecated worker",
				slog.String("short_code", code),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *URLService) GetStats(ctx context.Context, code string, userID int64) (*models.URLStats, error) {
	logger := s.logger.With(
		slog.String("short_code", code),
		slog.Int64("user_id", userID),
	)

	if code == "" {
		logger.WarnContext(ctx, "stats request failed: empty code")
		return nil, ErrShortCodeRequired
	}

	stats, err := s.repo.GetStats(ctx, code, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			logger.DebugContext(ctx, "stats lookup: no rows found or unauthorized")
			return nil, ErrURLNotFound
		}
		if errors.Is(err, repository.ErrLinkExpired) {
			logger.WarnContext(ctx, "stats lookup: link is expired")
			return nil, ErrShortCodeExpired
		}

		logger.ErrorContext(ctx, "failed to fetch stats from repository", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	logger.DebugContext(ctx, "stats retrieved successfully")
	return stats, nil
}

func (s *URLService) StartCleanupWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	workerLogger := s.logger.With(slog.String("component", "cleanup_worker"))
	workerLogger.Info("worker started", slog.Duration("interval", interval))

	for {
		select {
		case <-ticker.C:
			count, err := s.repo.DeleteExpired(ctx)
			if err != nil {
				workerLogger.Error("periodic cleanup failed", slog.Any("error", err))
			} else if count > 0 {
				workerLogger.Info("cleanup cycle complete", slog.Int64("deleted_rows", count))
			} else {
				workerLogger.Debug("cleanup cycle complete: nothing to delete")
			}
		case <-ctx.Done():
			workerLogger.Info("worker stopping via context cancellation")
			return
		}
	}
}
func (s *URLService) DeleteURL(ctx context.Context, code string, userID int64) error {
	logger := s.logger.With(slog.String("code", code), slog.Int64("user_id", userID))

	if err := s.rdb.Del(ctx, code).Err(); err != nil {
		logger.WarnContext(ctx, "failed to invalidate cache during delete", slog.Any("error", err))
	}

	err := s.repo.DeleteByID(ctx, code, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			logger.DebugContext(ctx, "delete failed: link not found or unauthorized")
			return ErrURLNotFound
		}
		logger.ErrorContext(ctx, "failed to delete url from repository", slog.Any("error", err))
		return err
	}

	logger.InfoContext(ctx, "url deleted successfully")
	return nil
}

func (s *URLService) RecordClick(ctx context.Context, short_code, ip_address, user_agent, referrer string) error {
	url, err := s.repo.GetByCode(ctx, short_code)
	if err != nil {
		return err
	}

	go func() {
		countryCode := s.getCountryCode(ip_address)
		deviceType, isBot := s.parseUserAgent(user_agent)

		err := s.repo.RecordClicks(models.Click{
			URLID:       url.ID,
			IpAddress:   ip_address,
			CountryCode: countryCode,
			UserAgent:   user_agent,
			DeviceType:  deviceType,
			Referrer:    referrer,
			IsBot:       isBot,
		})
		if err != nil {
			s.logger.Error("failed to record click analytics in background",
				slog.String("code", short_code),
				slog.Any("error", err),
			)
		}
	}()
	return nil
}

func (s *URLService) ListURLClicks(ctx context.Context, code string, userID, limit int64) ([]*models.Click, error) {
	logger := s.logger.With(slog.String("code", code), slog.Int64("user_id", userID))

	url, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			return nil, ErrURLNotFound
		}
		return nil, err
	}

	if url.UserID != userID {
		logger.WarnContext(ctx, "unauthorized attempt to list clicks")
		return nil, ErrURLNotFound
	}

	return s.repo.ListClicks(ctx, url.ID, limit)
}

func (s *URLService) GetClickStats(ctx context.Context, code string, userID int64) (*models.ClickStat, error) {
	logger := s.logger.With(slog.String("code", code), slog.Int64("user_id", userID))

	url, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			return nil, ErrURLNotFound
		}
		return nil, err
	}

	if url.UserID != userID {
		logger.WarnContext(ctx, "unauthorized attempt to get click stats")
		return nil, ErrURLNotFound
	}

	return s.repo.GetClickStats(ctx, url.ID)
}

func (s *URLService) GenerateQRCode(ctx context.Context, code string) ([]byte, error) {
	url, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			return nil, ErrURLNotFound
		}
		return nil, err
	}

	redirectURL := fmt.Sprintf("%s/%s", s.config.BaseURL, url.ShortCode)
	png, err := qrcode.Encode(redirectURL, qrcode.Medium, 256)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to generate qr code",
			slog.String("code", code),
			slog.Any("error", err),
		)
		return nil, err
	}

	s.logger.DebugContext(ctx, "qr code generated", slog.String("code", code))
	return png, nil
}
