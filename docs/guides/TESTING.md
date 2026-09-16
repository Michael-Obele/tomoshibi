# Testing Guide

> How to run, write, and maintain tests for Tomoshibi.
> See [Documentation Index](INDEX.md) for related guides.

_If you are looking for the overall workflow for Svelte devs, please see the [Svelte Dev Workflow](SVELTE_DEV_WORKFLOW.md)._

---

## 🧠 Testing: JS vs Go Mental Map

If you are coming from the Svelte/JS ecosystem (Vitest/Jest/Playwright):

| Feature         | Svelte/JS (Vitest)         | Go (`testing` package)                                        |
| --------------- | -------------------------- | ------------------------------------------------------------- |
| **Test Runner** | `npm run test` or `vitest` | `go test ./...`                                               |
| **Assertions**  | `expect(a).toBe(b)`        | `if a != b { t.Errorf() }`                                    |
| **Mocking**     | `vi.fn()` / `vi.mock()`    | Interfaces + Mock Structs                                     |
| **UI Testing**  | Playwright (`test()`)      | Go doesn't test UI, only APIs                                 |
| **Hot Reload**  | `vitest --watch`           | Use [air](https://github.com/cosmtrek/air) or re-run manually |

---

## Quick Start

```bash
# Run all unit tests
go test ./internal/... ./pkg/... -v

# Run tests for a specific package
go test ./internal/scraper/... -v

# Run a specific test
go test ./internal/scraper/... -run TestShouldUseDynamic -v

# Run with race detection
go test ./internal/... ./pkg/... -race

# Run with coverage report
go test ./internal/... ./pkg/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

---

## Test Structure

Tests live alongside the code they test, following the Go convention:

```
internal/
├── api/
│   ├── handlers/
│   │   ├── scrape.go         → scrape_test.go
│   │   ├── search.go         → search_test.go
│   │   └── helpers_test.go   (shared test setup: logger init)
│   └── middleware/
│       └── logger.go         → logger_test.go
├── config/
│   └── config.go             → config_test.go
├── domain/
│   └── scraper.go            → scraper_test.go
├── scraper/
│   ├── heuristics.go         → heuristics_test.go
│   └── service.go            → service_test.go
├── search/
│   └── service.go            → service_test.go
├── worker/
│   └── tasks.go              → tasks_test.go
pkg/
└── logger/
    └── logger.go             → logger_test.go
test/
└── integration_test.go       (integration tests, separate package)
```

---

## Running Tests

### All Unit Tests

```bash
go test ./internal/... ./pkg/... -v -count=1
```

| Flag       | Purpose                                           |
| ---------- | ------------------------------------------------- |
| `-v`       | Verbose output (shows each test name and status)  |
| `-count=1` | Disables test caching                             |
| `-short`   | Skip long-running tests (if any are tagged)       |
| `-race`    | Enable race condition detection                   |
| `-timeout` | Override default 10m timeout (e.g. `-timeout 5m`) |

### By Package

```bash
# Domain models
go test ./internal/domain/... -v

# Scraper (heuristics + service)
go test ./internal/scraper/... -v

# HTTP handlers
go test ./internal/api/handlers/... -v

# Search service
go test ./internal/search/... -v

# Worker tasks
go test ./internal/worker/... -v

# Config
go test ./internal/config/... -v

# Logger
go test ./pkg/logger/... -v
```

### With Coverage

```bash
# Generate coverage profile
go test ./internal/... ./pkg/... -coverprofile=coverage.out -covermode=atomic

# View in terminal
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

---

## Test Categories

### 1. Unit Tests (No Dependencies)

The majority of tests are pure unit tests that require no external services:

- **Domain tests**: JSON serialization/deserialization, struct validation
- **Heuristics tests**: SPA detection logic
- **Config tests**: Default values, Redis URL construction
- **Logger tests**: Initialization with different log levels
- **Worker task tests**: Payload creation, backward compatibility

### 2. Handler Tests (Mock Dependencies)

Handler tests use mock implementations to avoid hitting real services:

- **Search handler**: Uses `MockSearchService` implementing `search.Service`
- **Scrape handler**: Uses `mockStaticScraper` implementing `domain.Scraper`
- **Middleware**: Uses standard `httptest` with Gin test contexts

### 3. Service Tests (Mock Dependencies)

- **Scraper service**: Uses `mockScraper` implementing `domain.Scraper` to test mode selection (static/dynamic/smart) and fallback behavior
- **Search service**: Tests constructor, domain extraction, and API key validation

### 4. Integration Tests

Located in `packages/api/test/integration_test.go`. These test the full HTTP flow:

```bash
# Run integration tests (requires mock server, no external deps)
go -C packages/api test ./test/... -v
```

> **Note**: Integration tests use `httptest.NewServer` with mock services. They do NOT require a running Redis or Brave API key.

---

## Writing New Tests

### Pattern: Table-Driven Tests

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "Basic case",
            input:    "hello",
            expected: "HELLO",
        },
        {
            name:     "Empty string",
            input:    "",
            expected: "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := MyFunction(tt.input)
            if result != tt.expected {
                t.Errorf("got %q, want %q", result, tt.expected)
            }
        })
    }
}
```

### Pattern: Mock Services

If your code depends on an interface, create a mock:

```go
type mockScraper struct {
    result *domain.ScrapeResult
    err    error
}

func (m *mockScraper) Scrape(ctx context.Context, url string) (*domain.ScrapeResult, error) {
    return m.result, m.err
}
```

### Helpers: TestMain

If tests in a package need shared setup (e.g. logger initialization):

```go
func TestMain(m *testing.M) {
    logger.Init("error") // Quiet logs during tests
    os.Exit(m.Run())
}
```

---

## CI / Pre-commit

Run this before pushing:

```bash
# Format
go fmt ./...

# Vet
go vet ./internal/... ./pkg/...

# Test
go test ./internal/... ./pkg/... -race -count=1

# Full check (format + vet + test)
go fmt ./... && go vet ./internal/... ./pkg/... && go test ./internal/... ./pkg/... -race -v -count=1
```

---

## Troubleshooting

| Problem            | Solution                                                           |
| ------------------ | ------------------------------------------------------------------ |
| Stale test results | Add `-count=1` to disable caching                                  |
| Logger nil panic   | Add `TestMain` with `logger.Init("error")`                         |
| Redis tests fail   | Redis-dependent tests are currently mocked; no Redis required      |
| Browser tests fail | Chromedp tests need Chrome installed; use `-short` to skip         |
| Flaky search tests | Search tests use mocks; real API tests need `BRAVE_SEARCH_API_KEY` |
