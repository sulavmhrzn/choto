include .env
export

.PHONY: run build test tidy migrate-up migrate-down

run:
	@echo "Starting Choto API..."
	go run ./cmd/api

build:
	@echo "Building binary..."
	go build -o bin/choto ./cmd/api

tidy:
	go mod tidy

migrate-up:
	migrate -path migrations -database $(DB_CONN) up

migrate-down:
	migrate -path migrations -database $(DB_CONN) down

dev: tidy migrate-up run