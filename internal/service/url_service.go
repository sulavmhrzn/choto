package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"slices"

	"github.com/sulavmhrzn/choto/internal/repository"
)

var (
	ErrInvalidScheme     = errors.New("the provided url contains an invalid scheme")
	ErrInvalidURL        = errors.New("the provided url is not valid")
	ErrURLNotFound       = errors.New("url not found")
	ErrShortCodeRequired = errors.New("short code is required")
)

type Shortener interface {
	Shorten(ctx context.Context, longURL string) (string, error)
	GetOriginalURL(ctx context.Context, code string) (string, error)
	GetStats(ctx context.Context, code string) (*repository.URLStats, error)
	TrackClick(code string)
}

type URLRepository interface {
	Create(ctx context.Context, longURL string) (string, error)
	GetByCode(ctx context.Context, code string) (string, error)
	IncrementClick(code string) error
	GetStats(ctx context.Context, code string) (*repository.URLStats, error)
}

type URLService struct {
	repo URLRepository
}

func NewURLService(repo URLRepository) Shortener {
	return &URLService{
		repo: repo,
	}
}

func (s *URLService) Shorten(ctx context.Context, longURL string) (string, error) {
	validSchemas := []string{"http", "https"}
	u, err := url.ParseRequestURI(longURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ErrInvalidURL
	}
	if !slices.Contains(validSchemas, u.Scheme) {
		return "", ErrInvalidScheme
	}
	return s.repo.Create(ctx, longURL)
}

func (s *URLService) GetOriginalURL(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrShortCodeRequired
	}
	longURL, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNoRows) {
			return "", ErrURLNotFound
		}
		return "", fmt.Errorf("lookup failed for code %s: %w", code, err)
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
		if errors.Is(err, repository.ErrNoRows) {
			return nil, ErrURLNotFound
		}
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}
	return stats, nil

}
