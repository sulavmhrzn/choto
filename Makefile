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

swagger:
	swag init -g cmd/api/main.go --parseDependency --parseInternal


dev: tidy migrate-up run