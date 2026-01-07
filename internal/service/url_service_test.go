package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/sulavmhrzn/choto/internal/repository"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, longURL string, expiresAt *time.Time) (*repository.URL, error) {
	args := m.Called(ctx, longURL)
	return args.Get(0).(*repository.URL), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (string, error) {
	args := m.Called(ctx, code)
	return args.String(0), args.Error(1)
}

func (m *MockRepository) IncrementClick(code string) error {
	args := m.Called(code)
	return args.Error(0)
}

func (m *MockRepository) GetStats(ctx context.Context, code string) (*repository.URLStats, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.URLStats), args.Error(1)
}

func TestURLService_Shorten(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	mockRepo.On("Create", mock.Anything, "https://google.com").Return("abc123", nil)
	code, err := svc.Shorten(context.Background(), "https://google.com", nil)

	assert.NoError(t, err)
	assert.Equal(t, "abc123", code)
	mockRepo.AssertExpectations(t)
}

func TestURLService_Shorten_InvalidURL(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	code, err := svc.Shorten(context.Background(), "not-a-url", nil)

	assert.Error(t, err)
	assert.Empty(t, code)
	assert.Equal(t, ErrInvalidURL.Error(), err.Error())
}
func TestURLService_Shorten_InvalidSchema(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	code, err := svc.Shorten(context.Background(), "ftp://something.com", nil)

	assert.Error(t, err)
	assert.Empty(t, code)
	assert.Equal(t, ErrInvalidScheme.Error(), err.Error())
}

func TestURLService_GetByCode(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)
	mockRepo.On("GetByCode", mock.Anything, "abc123").Return("http://google.com", nil)

	url, err := svc.GetOriginalURL(context.Background(), "abc123")
	assert.NoError(t, err)
	assert.Equal(t, "http://google.com", url)
	mockRepo.AssertExpectations(t)
}

func TestURLService_GetStats(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	expectedStats := &repository.URLStats{
		LongURL:   "https://google.com",
		ShortCode: "abc123",
		Clicks:    5,
	}

	mockRepo.On("GetStats", mock.Anything, "abc123").Return(expectedStats, nil)

	stats, err := svc.GetStats(context.Background(), "abc123")

	assert.NoError(t, err)
	assert.Equal(t, 5, stats.Clicks)
	assert.Equal(t, "https://google.com", stats.LongURL)
}

func TestURLService_GetStats_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	mockRepo.On("GetStats", mock.Anything, "missing").Return(nil, repository.ErrNoRows)

	stats, err := svc.GetStats(context.Background(), "missing")

	assert.Nil(t, stats)
	assert.True(t, errors.Is(err, ErrURLNotFound))
}

func TestURLService_TrackClick(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	mockRepo.On("IncrementClick", "abc123").Return(nil)

	svc.TrackClick("abc123")

	time.Sleep(10 * time.Millisecond)

	mockRepo.AssertCalled(t, "IncrementClick", "abc123")
}
