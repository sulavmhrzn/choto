# Choto 🔗
**A high-performance URL shortener built with Go, PostgreSQL, and Redis.**

Choto is a lightweight, scalable URL shortening service designed with performance and security in mind. It demonstrates the use of Go concurrency, caching strategies, and rate-limiting.

## Features
- Fast Redirection: Uses a "Fire and Forget" strategy for analytics to ensure zero-latency redirects.
- Analytics: Tracks click counts for every shortened URL.
- Rate Limiting: Protects the API from abuse using a Redis-backed fixed-window algorithm.
- Performance: Implements Redis for caching and rate-limiting to reduce database load.
  
## Tech Stack
- Language: Go (Golang) 1.25+
- Web Framework: Gin Gonic
- Database: PostgreSQL (Persistence)
- Cache/Rate Limiter: Redis
- Configuration: Viper & Go-Playground Validator

## System Architecture
Choto uses a layered architecture to maintain clean code and separation of concerns:
- Handler Layer: Manages HTTP requests and responses.
- Service Layer: Contains business logic and orchestrates background tasks.
- Repository Layer: Handles all database and cache interactions.

#### Performance Optimization: Non-blocking Analytics
When a user visits a short link, the system redirects them immediately. The click-tracking logic runs in a separate Goroutine using a detached context. This ensures that database write latency never impacts the user experience.

## Getting Started
Prerequisites: 
- Go 1.23+
- PostgreSQL
- Redis

### Installation
1. Clone the repository
   ```sh
   git clone https://github.com/sulavmhrzn/choto/
   cd choto
   ```
2. Download and move [GeoLite2-Country.mmdb](https://github.com/P3TERX/GeoLite.mmdb/releases/download/2026.01.10/GeoLite2-Country.mmdb) in the project root
3. Create a .env file
   ```sh
   cp .env.example .env
   ```
   ```dosini
    ENV=development
    SERVER_PORT=8000
    DB_CONN=postgres://user:password@localhost:5432/choto?sslmode=disable
    DB_USER=postgres
    DB_PASSWORD=postgres
    DB_NAME=choto_db
    REDIS_ADDR=localhost:6379
    BASE_URL=http://localhost:8000
    RATE_LIMIT_COUNT=10
    SECRET_KEY=your-secret-key
   ```
4. Run the application
   ```sh
   make run
   ```

### Using Docker
- Start all services:
   ```sh
   docker compose up --build
   ```
- Stop all services:
  ```sh
  docker compose down
  ```
- Stop and remove volumes:
   ```sh
   docker compose down -v
   ```

### Future Roadmap
- [x] Docker & Docker Compose setup
- [x] Custom slug support
- [x] URL expiration dates
- [x] User authentication and dashboard
- [x] Graceful Shutdown
- [ ] Prometheus metrics and structured JSON logging.
- [ ] Use Redis to cache "hot" URLs and reduce DB load.
- [x] Integrated Swagger/OpenAPI UI.
- [ ] CI/CD pipeline using GitHub Actions to auto-deploy to a VPS.