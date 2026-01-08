package repository

import "errors"

var (
	ErrNoRows          = errors.New("no records found")
	ErrLinkExpired     = errors.New("link has expired")
	ErrUniqueShortCode = errors.New("short code taken")
	ErrDuplicateEmail  = errors.New("duplicate email")
)
