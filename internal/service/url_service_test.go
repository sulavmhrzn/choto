package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, longURL string) (string, error) {
	args := m.Called(ctx, longURL)
	return args.String(0), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (string, error) {
	args := m.Called(ctx, code)
	return args.String(0), args.Error(1)
}

func TestURLService_Shorten(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	mockRepo.On("Create", mock.Anything, "https://google.com").Return("abc123", nil)
	code, err := svc.Shorten(context.Background(), "https://google.com")

	assert.NoError(t, err)
	assert.Equal(t, "abc123", code)
	mockRepo.AssertExpectations(t)
}

func TestURLService_Shorten_InvalidURL(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	code, err := svc.Shorten(context.Background(), "not-a-url")

	assert.Error(t, err)
	assert.Empty(t, code)
	assert.Equal(t, ErrInvalidURL.Error(), err.Error())
}
func TestURLService_Shorten_InvalidSchema(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewURLService(mockRepo)

	code, err := svc.Shorten(context.Background(), "ftp://something.com")

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
