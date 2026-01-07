package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"slices"
	"time"

	"github.com/sulavmhrzn/choto/internal/repository"
)

var (
	ErrInvalidScheme     = errors.New("the provided url contains an invalid scheme")
	ErrInvalidURL        = errors.New("the provided url is not valid")
	ErrURLNotFound       = errors.New("url not found")
	ErrShortCodeRequired = errors.New("short code is required")
	ErrShortCodeExpired  = errors.New("short code has already expired")
)

type Shortener interface {
	Shorten(ctx context.Context, longURL string, expiresAt *time.Time) (*repository.URL, error)
	GetOriginalURL(ctx context.Context, code string) (string, error)
	GetStats(ctx context.Context, code string) (*repository.URLStats, error)
	TrackClick(code string)
	StartCleanupWorker(ctx context.Context, interval time.Duration)
}

type URLRepository interface {
	Create(ctx context.Context, longURL string, expiresAt *time.Time) (*repository.URL, error)
	GetByCode(ctx context.Context, code string) (string, error)
	IncrementClick(code string) error
	GetStats(ctx context.Context, code string) (*repository.URLStats, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

type URLService struct {
	repo URLRepository
}

func NewURLService(repo URLRepository) Shortener {
	return &URLService{
		repo: repo,
	}
}

func (s *URLService) Shorten(ctx context.Context, longURL string, expiresAt *time.Time) (*repository.URL, error) {
	validSchemas := []string{"http", "https"}
	u, err := url.ParseRequestURI(longURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, ErrInvalidURL
	}
	if !slices.Contains(validSchemas, u.Scheme) {
		return nil, ErrInvalidScheme
	}
	return s.repo.Create(ctx, longURL, expiresAt)
}

func (s *URLService) GetOriginalURL(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrShortCodeRequired
	}
	longURL, err := s.repo.GetByCode(ctx, code)
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
	if longURL == "" {
		return "", ErrURLNotFound
	}
	return longURL, nil
}

func (s *URLService) TrackClick(code string) {
	go func() {
		err := s.repo.IncrementClick(code)
		if err != nil {
			log.Printf("could not increment click for %s: %v", code, err)
		}
	}()
}

func (s *URLService) GetStats(ctx context.Context, code string) (*repository.URLStats, error) {
	if code == "" {
		return nil, ErrShortCodeRequired
	}
	stats, err := s.repo.GetStats(ctx, code)
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

	slog.Info("Cleanup worker started", "interval", interval)

	for {
		select {
		case <-ticker.C:
			count, err := s.repo.DeleteExpired(ctx)
			if err != nil {
				slog.Error("failed to delete expired URLs", "err", err)
			} else {
				slog.Info("Cleanup successful", "deleted_rows", count)
			}
		case <-ctx.Done():
			slog.Info("Cleanup worker stopping...")
			return
		}
	}
}
