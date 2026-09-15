# Actionable Todos

## Phase 1: Setup & Static Scraping

- [x] **Init Project**: Run `go mod init github.com/standard-user/tomoshibi`.
- [x] **Install Deps**: `go get -u github.com/gin-gonic/gin github.com/gocolly/colly/v2 github.com/spf13/viper github.com/brianvoe/gofakeit/v6`.
- [x] **Config**: Create `internal/config` package to load `env` variables using Viper.
- [x] **Logger**: Set up a structured logger (slog or zap) in `pkg/logger`.
- [x] **Scraper Interface**: Define the `Scraper` interface in `internal/domain`.
- [x] **Colly Impl**: Implement the static scraper in `internal/scraper/colly.go`.
  - [x] Configure User-Agent rotation using `gofakeit`.
  - [x] Add `html-to-markdown` conversion.
- [x] **API Handler**: Create `internal/api/handlers/scrape.go`.
- [x] **Router**: Wire up `POST /v1/scrape` in `internal/api/router.go`.
- [x] **Test**: Verify scraping a simple HTML page (e.g., `http://example.com`).

## Phase 2: Dynamic Scraping (Chromedp)

- [x] **Install Deps**: `go get -u github.com/chromedp/chromedp`.
- [x] **Chromedp Impl**: Implement dynamic scraper in `internal/scraper/chromedp.go`.
  - [x] Setup `chromedp.NewContext`.
  - [x] Implement `chromedp.Navigate`, `chromedp.WaitVisible`, `chromedp.OuterHTML`.
- [x] **Smart Switch**: Update `internal/scraper/service.go` to choose between Colly/Chromedp based on `render: true` flag.
- [x] **Docker**: Create `Dockerfile` with Chromium installation (see `architecture.md`).
- [x] **Test**: Verify scraping a React site (e.g., a dynamic todo app).

## Phase 3: Async Queue (Asynq)

- [x] **Install Deps**: `go get -u github.com/hibiken/asynq`.
- [x] **Redis Setup**: Configure `asynq.RedisClientOpt` in `internal/config` (ensure TLS support).
- [x] **Task Definition**: Create `internal/worker/tasks.go` (define `TypeCrawl`).
- [x] **Task Handler**: Create `internal/worker/handlers.go` (logic to call Scraper Service).
- [x] **Server**: Create `cmd/worker/main.go` to run the Asynq server.
- [x] **API Update**: Add `POST /v1/crawl` to enqueue tasks.
- [x] **Status Endpoint**: Add `GET /v1/crawl/:id` to query job status (implemented using Asynq Inspector).

## Phase 4: Polish & Auth

- [x] **Middleware**: Implement `APIKeyAuth` middleware in `internal/api/middleware/auth.go`.
- [x] **Apply Middleware**: Protect `/v1/*` routes in `router.go`.
- [x] **Rate Limiting**: Per-client Redis (or in-memory) limiter via `RATE_LIMIT_RPM`.
- [x] **Cleanup**: Ensure `defer cancel()` is called on all contexts to prevent memory leaks.
- [x] **Documentation**: Generate Swagger/OpenAPI spec if needed.

## Phase 5: High Performance & Reliability (Leapcell/Upstash)

- [x] **Refactor Scraper**: Move `chromedp` Allocator to a specific Service/Singleton to reuse the browser process.
- [x] **Tuning**: Parallel crawl pool (`CRAWL_CONCURRENCY`) and per-domain politeness (`CRAWL_DOMAIN_DELAY`).
- [x] **Smart Waiting**: Page actions (`wait_ms`, `wait_selector`) + settle loop for `scroll_to_bottom`.
- [x] **Stability**: Periodic browser restarts (`CHROME_RECYCLE_AFTER`, default 100) to prevent memory leaks.
- [x] **Resilience**: Tune Redis timeouts for high-latency environments.

## Phase 6: Tomoshibi v2 Feature Sprint (2026-08-01)

- [x] **Core Quality**: readability main-content extraction, full-options cache keys, ScreenshotOptions, UA rotation, parallel blob fetch, browser recycling.
- [x] **Image Engine v2**: srcset/lazy/picture extraction, optimizer unwrap, quality ranking, dimension sniffing, resize/re-encode.
- [x] **Crawl v2**: parallel workers, per-domain politeness, retry policy (4xx skip), include/exclude patterns, signed webhooks.
- [x] **Discovery & Endpoints**: `/v1/map`, `/v1/batch/scrape`, page actions.
- [x] **Intelligence (no LLM)**: deterministic schema extraction, extractive summary, PII redaction, change-tracking monitors, ad/base64 defaults.
- [x] **Hardening**: API-key auth + rate limiting (Phase 4 closed).
