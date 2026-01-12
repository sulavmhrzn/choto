package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/mileusna/useragent"
	"github.com/oschwald/geoip2-golang"
	"github.com/redis/go-redis/v9"
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
	Shorten(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*repository.URL, error)
	GetOriginalURL(ctx context.Context, code string) (string, error)
	GetStats(ctx context.Context, code string, userID int64) (*repository.URLStats, error)
	DeprecatedTrackClick(code string)
	StartCleanupWorker(ctx context.Context, interval time.Duration)
	DeleteURL(ctx context.Context, code string, userID int64) error
	RecordClick(ctx context.Context, short_code, ip_address, user_agent, referrer string) error
}

type URLRepository interface {
	Create(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*repository.URL, error)
	GetByCode(ctx context.Context, code string) (*repository.URL, error)
	IncrementClick(code string) error
	GetStats(ctx context.Context, code string, userID int64) (*repository.URLStats, error)
	DeleteExpired(ctx context.Context) (int64, error)
	DeleteByID(ctx context.Context, code string, userID int64) error
	RecordClicks(click repository.Click) error
}

type URLService struct {
	repo   URLRepository
	rdb    *redis.Client
	logger *slog.Logger
}

func NewURLService(repo URLRepository, rdb *redis.Client, logger *slog.Logger) Shortener {
	return &URLService{
		repo:   repo,
		rdb:    rdb,
		logger: logger,
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

func (s *URLService) Shorten(ctx context.Context, longURL string, expiresAt *time.Time, alias string, userID int64) (*repository.URL, error) {
	cleanURL := strings.TrimSpace(longURL)
	u, err := url.ParseRequestURI(cleanURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, ErrInvalidURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	finalURL := u.String()

	if !s.IsValidScheme(u.Scheme) {
		return nil, ErrInvalidScheme
	}
	if alias != "" && s.IsReserved(alias) {
		return nil, ErrReservedAlias
	}
	alias = strings.TrimSpace(strings.ToLower(alias))
	createdURL, err := s.repo.Create(ctx, finalURL, expiresAt, alias, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUniqueShortCode) {
			return nil, ErrAliasAlreadyTaken
		}
		return nil, err
	}
	return createdURL, nil
}

func (s *URLService) GetOriginalURL(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrShortCodeRequired
	}
	url, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNoRows):
			return "", ErrURLNotFound
		case errors.Is(err, repository.ErrLinkExpired):
			return "", ErrShortCodeExpired
		default:
			return "", fmt.Errorf("lookup failed for code %s: %w", code, err)
		}
	}
	if url.LongURL == "" {
		return "", ErrURLNotFound
	}
	return url.LongURL, nil
}

func (s *URLService) DeprecatedTrackClick(code string) {
	go func() {
		err := s.repo.IncrementClick(code)
		if err != nil {
			s.logger.Error("could not increment click", "code", code, "err", err)
		}
	}()
}

func (s *URLService) GetStats(ctx context.Context, code string, userID int64) (*repository.URLStats, error) {
	if code == "" {
		return nil, ErrShortCodeRequired
	}
	stats, err := s.repo.GetStats(ctx, code, userID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNoRows):
			return nil, ErrURLNotFound
		case errors.Is(err, repository.ErrLinkExpired):
			return nil, ErrShortCodeExpired
		default:
			return nil, fmt.Errorf("failed to get stats: %w", err)
		}
	}
	return stats, nil

}

func (s *URLService) StartCleanupWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info("Cleanup worker started", "interval", interval)

	for {
		select {
		case <-ticker.C:
			count, err := s.repo.DeleteExpired(ctx)
			if err != nil {
				s.logger.Error("failed to delete expired URLs", "err", err)
			} else {
				s.logger.Info("Cleanup successful", "deleted_rows", count)
			}
		case <-ctx.Done():
			slog.Info("Cleanup worker stopping...")
			return
		}
	}
}

func (s *URLService) DeleteURL(ctx context.Context, code string, userID int64) error {
	s.rdb.Del(ctx, code)
	err := s.repo.DeleteByID(ctx, code, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			return ErrURLNotFound
		}
		return err
	}
	return nil
}

func (s *URLService) RecordClick(
	ctx context.Context,
	short_code,
	ip_address,
	user_agent,
	referrer string,
) error {
	url, err := s.repo.GetByCode(ctx, short_code)
	if err != nil {
		return err
	}
	go func() {
		countryCode := s.getCountryCode(ip_address)
		deviceType, isBot := s.parseUserAgent(user_agent)
		err := s.repo.RecordClicks(repository.Click{
			URLID:       url.ID,
			IpAddress:   ip_address,
			CountryCode: countryCode,
			UserAgent:   user_agent,
			DeviceType:  deviceType,
			Referrer:    referrer,
			IsBot:       isBot,
		})
		if err != nil {
			s.logger.Error("failed to record click", "err", err)
		}
	}()
	return nil
}

func (s *URLService) parseUserAgent(uaString string) (string, bool) {
	ua := useragent.Parse(uaString)
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
	db, err := geoip2.Open("GeoLite2-Country.mmdb")
	if err != nil {
		return "XX"
	}

	defer db.Close()

	ip := net.ParseIP(ipAddr)
	record, err := db.Country(ip)
	if err != nil {
		return "XX"
	}
	return record.Country.IsoCode
}
