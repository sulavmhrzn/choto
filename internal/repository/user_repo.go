package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

var (
	ErrDuplicateEmail = errors.New("duplicate email")
)

type UserRepository struct {
	DB *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{
		DB: db,
	}
}

type User struct {
	ID        int
	Email     string
	CreatedAt time.Time
}

func (r *UserRepository) Create(ctx context.Context, email string, passwordHash string) (*User, error) {
	query := `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, created_at`
	var user User
	err := r.DB.QueryRowContext(ctx, query, email, passwordHash).Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		if err, ok := err.(*pq.Error); ok {
			if err.Code == "23505" {
				return nil, ErrDuplicateEmail
			}
		}
		return nil, err
	}
	return &user, nil
}
