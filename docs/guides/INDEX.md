# 📚 Tomoshibi Documentation Index

Welcome to the Tomoshibi documentation! This guide is designed to help **full-stack Svelte/TypeScript developers** understand this Go codebase.

---

## Quick Start

If you're new to Go, read these in order:

| #   | Document                                      | Description                         | Time   |
| --- | --------------------------------------------- | ----------------------------------- | ------ |
| 1   | [Go for Svelte Devs](GO_FOR_SVELTE_DEVS.md)   | Mental models, comparisons to JS/TS | 20 min |
| 2   | [Go Syntax Reference](GO_SYNTAX_REFERENCE.md) | Quick reference for Go syntax       | 10 min |
| 3   | [Code Walkthrough](CODE_WALKTHROUGH.md)       | File-by-file annotated code         | 30 min |
| 4   | [Svelte Dev Workflow](SVELTE_DEV_WORKFLOW.md) | How to run, use, test, and debug    | 15 min |
| 5   | [Swagger Documentation](SWAGGER.md)           | Interactive API docs and schema     | 5 min  |

---

## 🏗️ Core Architecture & Features

| Document                                                      | Description                                     |
| ------------------------------------------------------------- | ----------------------------------------------- |
| [Architecture Guide](ARCHITECTURE.md)                         | High-level design and task queue patterns       |
| [Project Tour](PROJECT_TOUR.md)                               | Visual map of the codebase for JS devs          |
| [API Reference](API_REFERENCE.md)                             | Complete endpoint docs (scrape, search, crawl)  |
| [Performance Changes](CHANGELOG_JS.md)                        | How the browser context pool optimization works |
| [Testing Guide](TESTING.md)                                   | How to run and write Go tests                   |
| [SearXNG on Fly](SEARXNG_FLY.md)                               | Deploy the self-hosted search backend for /v1/search |
| **Features Explorer**                                         |                                                 |
| [Search](../search/service.go)                                | SearXNG (self-hosted, primary) + Brave fallback |
| [Screenshot & Images](../features/image-screenshot-feature.md)| Capturing page screenshots and images           |
| [Image Blob Transport](../features/image-blob-transport.md)   | AI-ready image pipeline (base64 data URIs)      |

---

## Documentation Overview

### [GO_FOR_SVELTE_DEVS.md](GO_FOR_SVELTE_DEVS.md)

**Audience**: Svelte/TypeScript developers new to Go

**Covers**:

- Go vs TypeScript mental model comparison
- Variable declarations (`:=` vs `var`)
- Functions and methods
- Structs (like TS interfaces/classes)
- Error handling (no try/catch!)
- Pointers explained simply
- Packages and imports
- Context (like SvelteKit's event)
- Goroutines (async/await equivalent)

### [SVELTE_DEV_WORKFLOW.md](SVELTE_DEV_WORKFLOW.md)

**Audience**: Svelte/TypeScript developers actively working on this project (tomoshibi or tomoshibi-js)

**Covers**:

- How to run the Go backend alongside a Svelte frontend
- How to consume the Go API in SvelteKit (`+page.server.ts`)
- How to run backend unit tests vs frontend Vitest
- How to debug the API using VS Code and `logger`

### [SWAGGER.md](SWAGGER.md)

**Audience**: Frontend developers integrating with the API

**Covers**:

- How to access the live Swagger UI playground
- Understanding endpoints and response shapes
- How to generate and update the static API schema with `swag init`

### [GO_SYNTAX_REFERENCE.md](GO_SYNTAX_REFERENCE.md)

**Audience**: Anyone needing a quick syntax lookup

**Covers**:

- Variable declarations and zero values
- Function signatures and returns
- Struct tags and methods
- Interface definitions
- Control flow (if, switch, for)
- Slices and maps
- Pointers
- Concurrency (goroutines, channels)
- JSON encoding/decoding
- Testing patterns
- Common idioms

### [CODE_WALKTHROUGH.md](CODE_WALKTHROUGH.md)

**Audience**: Developers wanting to understand the actual codebase

**Covers**:

- Entry points (`cmd/api/main.go`, `cmd/worker/main.go`)
- Configuration (`internal/config/`)
- HTTP routing and handlers (`internal/api/`)
- Business logic (`internal/scraper/`, `internal/search/`)
- Background jobs (`internal/worker/`)
- Shared utilities (`pkg/logger/`)

---

## Project Architecture

```
tomoshibi/
├── cmd/                    # Entry points (executables)
│   ├── api/main.go         # HTTP server + embedded worker
│   └── worker/main.go      # Standalone worker
├── internal/               # Private application code
│   ├── api/                # HTTP layer
│   │   ├── router.go       # Route definitions
│   │   ├── handlers/       # Request handlers
│   │   └── middleware/     # HTTP middleware
│   ├── config/             # Configuration loading
│   ├── domain/             # Core types & interfaces
│   ├── scraper/            # Scraping implementations
│   ├── search/             # Search service
│   └── worker/             # Job processing
├── pkg/                    # Public shared libraries
│   └── logger/             # Logging utilities
├── docs/                   # Documentation
│   └── guides/             # Developer guides (you are here!)
└── go.mod                  # Dependencies (like package.json)
```

---

## Key Concepts Map

| SvelteKit Concept              | Go Equivalent              | Tomoshibi Location                |
| ------------------------------ | --------------------------- | ------------------------------ |
| `+page.server.ts`              | Handler function            | `internal/api/handlers/*.go`  |
| `hooks.server.ts`              | Middleware                  | `internal/api/middleware/`     |
| Route group `(api)`            | Router group                | `v1 := r.Group("/v1")`         |
| `$lib`                         | `internal/`                 | `internal/*`                   |
| `types.ts`                     | Domain package              | `internal/domain/`             |
| Environment vars               | Viper config                | `internal/config/`             |
| `package.json`                 | `go.mod`                    | Project root                   |
| `npm run check`                | `make check`                | Project root (Makefile)        |
| `npm install`                  | `go mod download`           | —                              |
| External API proxy in server   | Search service              | `internal/search/service.go`   |
| File upload / base64 pipeline  | Image processor             | `internal/image/processor.go`  |

---

## Request Flow

```
HTTP Request
    │
    ▼
┌─────────────────────┐
│  router.go          │  ← Route matching
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  middleware/        │  ← Logging, recovery
│  logger.go          │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  handlers/          │  ← Parse request, call service
│  scrape.go          │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  scraper/           │  ← Business logic
│  service.go         │
└─────────┬───────────┘
          │
          ├─────────────────────────┐
          │                         │
          ▼                         ▼
┌─────────────────────┐   ┌─────────────────────┐
│  scraper/           │   │  scraper/           │
│  colly.go           │   │  chromedp.go        │
│  (static scraping)  │   │  (dynamic scraping) │
└─────────────────────┘   └─────────────────────┘
```

---

## Async Job Flow (Crawl)

```
POST /v1/crawl
    │
    ▼
┌─────────────────────┐
│  handlers/crawl.go  │
│  EnqueueCrawl()     │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Redis Queue        │  ← Job stored
│  (Asynq)            │
└─────────┬───────────┘
          │
          │ (async, later)
          ▼
┌─────────────────────┐
│  worker/handlers.go │
│  ProcessTask()      │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  scraper/service.go │
│  Scrape()           │
└─────────────────────┘
```

---

## Quick Commands

_For a more detailed breakdown tailored to JS/Svelte developers, see [Svelte Dev Workflow](SVELTE_DEV_WORKFLOW.md)._

```bash
# Development
go run ./cmd/api          # Start server (Monolith API + worker)
go run ./cmd/worker       # Start standalone worker (Optional now, API embeds it)
```

```bash
# Building
go build -o bin/api ./cmd/api
go build -o bin/worker ./cmd/worker
```

```bash
# Testing (See TESTING.md for full guide)
go test ./...             # All tests
go test -v ./...          # Verbose
go test -cover ./...      # With coverage
```

```bash
# Dependencies
go mod download           # Install deps (like npm install)
go mod tidy               # Clean up go.mod (remove unused, add used)
```

```bash
# Code quality
go fmt ./...              # Format code (like Prettier)
go vet ./...              # Static analysis (like ESLint)
```

---

## Environment Variables

| Variable               | Default | Description                      |
| ---------------------- | ------- | -------------------------------- |
| `PORT`                 | `8080`  | HTTP server port                 |
| `SERVER_MODE`          | `debug` | `debug`, `release`, `test`       |
| `LOG_LEVEL`            | `info`  | `debug`, `info`, `warn`, `error` |
| `REDIS_URL`            | —       | Redis connection URL             |
| `BRAVE_SEARCH_API_KEY` | —       | Brave Search API key             |
| `DISABLE_WORKER`       | `false` | Disable embedded worker          |

---

## Further Reading

- [Official Go Tour](https://go.dev/tour/) – Interactive Go tutorial
- [Effective Go](https://go.dev/doc/effective_go) – Go best practices
- [Go by Example](https://gobyexample.com/) – Annotated code examples
- [Gin Documentation](https://gin-gonic.com/docs/) – HTTP framework used
- [Asynq Documentation](https://github.com/hibiken/asynq) – Job queue library
