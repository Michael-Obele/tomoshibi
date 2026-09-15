# Tomoshibi 🔥 - Gemini Context (formerly Cinder)

Tomoshibi is a high-performance, self-hosted web scraping API built with Go, designed as a drop-in alternative to Firecrawl. It converts complex websites into LLM-ready markdown.

## 🏗️ Project Overview

- **Core Tech:** Go 1.25+, Gin (API), Chromedp (Dynamic), Colly (Static), Asynq/Redis (Async Queue), Brave Search.
- **Architecture:** Monolithic with Embedded Worker. It runs the API and background worker in a single binary, optimized for serverless and hobby-tier environments.
- **Modes:**
  - `static`: Uses Colly for fast, lightweight HTML parsing.
  - `dynamic`: Uses Chromedp for JavaScript rendering.
  - `smart`: Auto-detects and falls back to dynamic if static scraping is insufficient.

## 📁 Key Directory Structure

- `cmd/api/`: Entry point for the monolith API and worker.
- `internal/api/`: Gin router, handlers, and middleware.
- `internal/scraper/`: Core scraping services and "smart" selection logic.
- `internal/worker/`: Asynq task definitions and server setup.
- `internal/domain/`: Core data structures and interfaces.
- `internal/config/`: Configuration management using Viper/Godotenv.
- `pkg/logger/`: Centralized structured logging (slog).
- `docs/`: Extensive project documentation and feature specs.
  Previously `cinder-js/` & `cinder-js-gpt/` — archived.

## 🚀 Building and Running

### Development

```bash
# Install dependencies
go mod download

# Run the API (includes embedded worker by default)
go run ./cmd/api
```

### Docker

```bash
# Build the image
docker build -t cinder .

# Run the container
docker run -p 8080:8080 -e SERVER_MODE=release cinder
```

### Testing

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./internal/scraper/...
```

## 🛠️ Configuration

Configuration is handled via environment variables or a `.env` file:

- `PORT`: Server port (default: 8080)
- `SERVER_MODE`: `debug`, `release`, or `test`
- `REDIS_URL`: Required for asynchronous crawling (`/v1/crawl`)
- `BRAVE_SEARCH_API_KEY`: Required for the `/v1/search` endpoint
- `DISABLE_WORKER`: Set to `true` to disable the embedded background worker

## 📝 Development Conventions

- **Logging:** Always use `pkg/logger` for structured logging.
- **Errors:** Handle errors explicitly; avoid silent failures. Use descriptive error messages.
- **Interfaces:** Define interfaces in `internal/domain` to keep the core logic decoupled from implementation details.
- **Swagger:** API documentation is auto-generated using `swag`. In debug mode, the API attempts to re-generate docs on startup.
- **Concurrency:** The worker is configured for 10 concurrent jobs by default (adjustable in `internal/worker/server.go`).

## 🗺️ Roadmap Focus

- Increasing test coverage (currently low).
- Implementing "Smart Wait" heuristics for SPAs.
- Enhancing browser health monitoring to prevent memory leaks.
