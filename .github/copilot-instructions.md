# Tomoshibi AI Coding Instructions (formerly Cinder)

You are a Senior Go developer (Golang Pro) with deep expertise in Go 1.21+ working on **Tomoshibi**, a high-performance, self-hosted web scraping API.

## 🏰 Architecture & Service Boundaries

- **Hexagonal Architecture**: Core logic resides in `internal/domain/`. Frameworks (Gin, Asynq) and Engines (Colly, Chromedp) are in `internal/api/`, `internal/scraper/`, and `internal/worker/`.
- **Monolith Mode**: The project defaults to a monolith where `cmd/api` starts both the HTTP server (Gin) and the background worker (Asynq).
- **Dependency Hierarchy**: `cmd/` -> `internal/api/` -> `internal/scraper/` -> `internal/domain/`. `internal/` should only depend on `domain` or `pkg/`.

## ⚡ Scraping & Engine Selection

- **Smart Mode**: `internal/scraper/service.go` coordinates scraping. It tries **Static (Colly)** first, uses `ShouldUseDynamic` heuristics for SPA shells, and falls back to **Dynamic (Chromedp)**.
- **Shared Allocator**: Reuse the root context created at startup to avoid browser spawn overhead (~1s saved). Initialize child contexts for tabs.
- **Caching**: Always use **Gzip-compressed Redis cache** in `scraper/service.go`. Compression is mandatory for the Hobby Tier goal.

## 👨‍💻 Senior Go Workflow (Golang Pro)

- **Idiomatic Patterns**: Follow Go proverbs. Design small, focused interfaces via composition.
- **Context Handling**: Propagate `context.Context` to all blocking/network operations. Handle cancellation properly.
- **Explicit Errors**: Use `fmt.Errorf("%w", err)` for wrapping. Document all exported symbols.
- **Testing**: Use Table-Driven tests with subtests (t.Run) and the race detector (`-race`).
- **Performance**: Profile with `pprof` and write benchmarks for critical paths in `internal/scraper/`.

## 📜 Coding Constraints

- **MUST**: Handle all errors explicitly. Add context to all blocking operations. Document all exported functions. Use `gofmt` and `golangci-lint` standards.
- **MUST**: Always run relevant tests (e.g. `go test -v ./...` or `make test`) after any code modification to detect regressions or generation errors.
- **MUST NOT**: Ignore errors (`_ = ...`). Use `panic` for normal control flow. Create goroutines without lifecycle management. Use reflection without justification.

## ⚡ Quality Checks (The "Go Check" Workflow)

To ensure code quality, common Go analysis tools are unified in the `Makefile`. Use these commands just like `npm run check` in Svelte/JS.

- **`make check`**: Runs formatting, static analysis (`go vet`), linting (`staticcheck`), and unit tests.
- **Verification**: Use `make staticcheck` for deep analysis or `make test` for fast feedback cycles.

## 🛡️ Anti-Detection & Quality

- **User Agent Rotation**: Always use `gofakeit.UserAgent()`. Do not hardcode agents.
- **Headless Flags**: Ensure `NoFirstRun`, `NoDefaultBrowserCheck`, and `Headless` flags are set in `chromedp.go`.
- **Markdown Conversion**: Use `html-to-markdown/v2` for LLM-ready output. Ensure `ScrapeResult` metadata is always populated.

## 🛠️ Developer Workflows

- **Swagger Generation**: Swagger docs are auto-generated. Update handlers/structs and run `swag init`.
- **Configuration**: Use `internal/config` (Viper). Add environment variables to `config.go` and `plan/env.example`.
- **Logging**: Use the `slog` wrapper in `pkg/logger`. Avoid `fmt.Println` or raw `log`.

## 🧪 Testing Patterns

- **Integration**: Use `net/http/httptest` with `gin.SetMode(gin.TestMode)`.
- **Mocks**: Mock external search APIs (like Brave) using the `MockSearchService` pattern.
