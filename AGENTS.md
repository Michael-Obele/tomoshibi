# Tomoshibi 🔥 - Agent Context (formerly Cinder)

Tomoshibi is a high-performance, self-hosted web scraping API built with Go, designed as a drop-in alternative to Firecrawl. It converts complex websites into LLM-ready markdown.

**Single source of truth:** every coding agent reads this file (Claude Code via the `CLAUDE.md` symlink, Gemini/OpenCode/Copilot via `AGENTS.md`). Edit here only — never write a second copy.

## 🏗️ Project Overview

- **Core Tech:** Go 1.25+, Gin (API), Chromedp (Dynamic), Colly (Static), Asynq/Redis (Async Queue), Brave Search.
- **Architecture:** Monolithic with Embedded Worker. It runs the API and background worker in a single binary, optimized for serverless and hobby-tier environments.
- **Modes:**
  - `static`: Uses Colly for fast, lightweight HTML parsing.
  - `dynamic`: Uses Chromedp for JavaScript rendering.
  - `smart`: Auto-detects and falls back to dynamic if static scraping is insufficient.

Tomoshibi = self-hosted web scrape API in Go (formerly Cinder). Ship as **monolith with embedded worker**: `cmd/api` runs both Gin HTTP server and Asynq background worker in one process, sized for 512MB–1GB hobby-tier host.

## 📁 Key Directory Structure

All paths relative to `packages/api/` unless prefixed with `packages/`.

- `cmd/api/`: Entry point for the monolith API and worker.
- `internal/api/`: Gin router, handlers, and middleware.
- `internal/scraper/`: Core scraping services and "smart" selection logic.
- `internal/search/`: Search backends (`Service` chain) + in-house engine layer:
  - `engines/` — declarative YAML roster (`registry.default.yaml`), css/rss/json parsers, TLS/proxy transport pool
  - `compat/` — SearXNG JSON contract listener + golden fixtures
- `internal/worker/`: Asynq task definitions and server setup.
- `internal/domain/`: Core data structures and interfaces.
- `internal/config/`: Configuration management using Viper/Godotenv.
- `internal/{extract,image,sitemap,safeurl}/`: Support packages (schema extraction/summary/PII redaction, image pipeline, sitemap discovery, SSRF guard).
- `pkg/logger/`: Centralized structured logging (slog).
- `packages/mcp/`, `packages/web/`: TypeScript MCP server and Svelte 5 playground.
- `docs/`: Local-only project documentation (gitignored except `SEARCH_COMPARISON.md`, `guides/API_REFERENCE.md`, `guides/SEARXNG_FLY.md`).
  Previously `cinder-js/` & `cinder-js-gpt/` — archived.
- `plan/`: Local-only design plans (gitignored), incl. `plan/tomoshi-search/` (search backend plan + probe log).

## 🚀 Building and Running

Commands below run from `packages/api/`.

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
make check        # fmt + vet + staticcheck + test — run this before finishing work
make test         # go test -v ./...
make fmt          # go fmt ./...
make vet          # go vet ./...
make staticcheck  # requires: go install honnef.co/go/tools/cmd/staticcheck@latest
```

```bash
go run ./cmd/api                                          # API + embedded worker
DISABLE_WORKER=true go run ./cmd/api                      # API only
go run ./cmd/worker                                       # standalone worker (exits 0 if no REDIS_URL)

go test ./internal/scraper/... -v                         # one package
go test ./internal/scraper/... -run TestShouldUseDynamic  # one test
go test ./internal/... ./pkg/... -race -count=1           # race, no cache
go test ./test/... -v                                     # integration (package `integration`, mocks only)
go test ./internal/... ./pkg/... -coverprofile=coverage.out && go tool cover -func=coverage.out
```

`docker-compose.yml` bring up Redis too. Deploy target = Fly.io via `fly.toml`.

Test need no Redis, no Brave key — all external thing mocked. Chromedp test need local Chromium.

## 🛠️ Configuration

Configuration is handled via environment variables or a `.env` file (canonical reference: `packages/api/.env.example`):

- `SERVER_PORT`: Server port (default: 7431; Fly injects `PORT`)
- `SERVER_MODE`: `debug`, `release`, or `test`
- `REDIS_URL`: Required for asynchronous crawling (`/v1/crawl`)
- `BRAVE_SEARCH_API_KEY`: Brave Search API fallback (free $5 credit/mo ≈ 1,000 searches)
- `SEARCH_NATIVE_ENABLED` / `SEARCH_COMPAT_ADDR` / `SEARCH_ENGINES_PATH`: in-house search layer (see **Search backends** below)
- `WEBSHARE_API_KEY` / `WEBSHARE_PROXY_URL` / `SEARCH_PROXY_TIER1|T2` / `SEARCH_PROXY_BUDGET_GB`: search proxy groups
- `DISABLE_WORKER`: Set to `true` to disable the embedded background worker

## Architecture

Dependency go one way: `cmd/` → `internal/api/` → `internal/scraper/` → `internal/domain/`. Interface live in `internal/domain` (`domain.Scraper` = the seam), so `internal/*` reach only for `domain` or `pkg/`. `internal/search` fully standalone — import nothing from project, define own `Service` interface and types.

**Startup wiring** (`cmd/api/main.go`) = map of system: config → logger → optional Redis client → Colly scraper + Chromedp scraper → `scraper.Service` → handlers → optional crawl handler + embedded worker + monitor scheduler → router.

**Engine selection** (`internal/scraper/service.go:55`) = core decision point:

- `static` → Colly. `dynamic` → Chromedp. `smart` (default) → Colly first, then `ShouldUseDynamic(html)` (`heuristics.go`, SPA-shell marker + content-size check) decide whether redo in Chromedp.
- Page `actions` force dynamic, and `static` + actions = hard error. `screenshot` in smart mode go straight to dynamic.
- Post-scrape enrichment run in fixed order: schema extraction → summary → PII redaction → image extraction/blob fetch (bounded `errgroup`, limit 5).

**Caching** = gzip-compressed JSON in Redis, 7-day TTL. Key = SHA-256 of URL + mode + _whole_ `ScrapeOptions` struct (`cacheKeyFor`), so add field to `ScrapeOptions` auto-prevent stale hit — no hand-roll narrower key. Compression deliberate (hobby-tier storage), and read path still tolerate legacy uncompressed value. `Service.cache` = two-method `cacheStore` interface, not `*redis.Client`, so test can drive cache branch; `NewService` still take concrete client and assign only when non-nil, because nil `*redis.Client` in interface is not nil interface.

**Browser lifetime** (`internal/scraper/chromedp.go`): one shared exec allocator for process, light tab per scrape, full allocator restart every `CHROME_RECYCLE_AFTER` scrape to bound Chrome memory growth. Never spawn browser per request. Allocator warm up sync at startup so missing Chromium show as startup warning and dynamic mode degrade instead of fail later.

**Async work** (`internal/worker/`): task type declared in `tasks.go` (`scrape:url`, `crawl:site`, `monitor:check`) and registered on mux in `server.go`. Asynq run concurrency 5 across `critical`/`default`/`low` queue with 15s `TaskCheckInterval` — both number tuned for 256MB VM and Upstash free command budget, so treat as intentional.

`crawl_handler.go:ExecuteCrawl` = parallel BFS with per-domain politeness delay, retry-with-backoff that never retry 4xx, and bounded queue that drop excess link instead of block producer. It enforce own deadline and return status `"timeout"`; Asynq task timeout deliberately `crawlTimeout() + 5m` as outer safety net. Seed URL always scraped — `include_paths`/`exclude_paths` apply only to discovered link.

Crawl tuning read from environment at call time via `clampEnvInt` helper at bottom of `crawl_handler.go` (`CRAWL_CONCURRENCY`, `CRAWL_DOMAIN_DELAY`, `CRAWL_TIMEOUT`, `CRAWL_SCRAPE_TIMEOUT`, `CRAWL_MAX_RETRIES`), not through `internal/config` — follow whichever pattern surrounding file already use.

**Graceful degradation = design rule.** Redis optional: without it, `/v1/crawl` return 503 and `/v1/batch` + `/v1/monitor` never registered (`internal/api/router.go:79`). Readability failure return raw HTML with nil error. Per-image fetch failure logged and skipped. Scrape must not fail because enrichment step did.

**SSRF defense** (`internal/safeurl`) = two layer, because either layer alone leak. `safeurl.Client`/`Transport`/`Dialer` install `net.Dialer.Control` hook that run _after_ DNS resolution, so it catch redirect and DNS rebinding; `safeurl.Check` = pre-flight URL check used only where we not own connection (chromedp drive real browser). Every outbound fetch path must go through one of them. Private, loopback, link-local, multicast and CGNAT address refused by default; `SSRF_ALLOW_PRIVATE=true` opt out for operator scraping internal wiki. Test that use `httptest` bind to 127.0.0.1 so need that variable set — package do it in `TestMain` (see `internal/scraper/main_test.go`), and test that exercise the guard itself override with `t.Setenv`.

**Shutdown** (`cmd/api/main.go`) drain HTTP server and embedded worker at same time, not in sequence: `signal.NotifyContext` fire → worker `Shutdown()` start in goroutine → `srv.Shutdown` run in foreground → `select` wait on whichever finish first, bounded by `SHUTDOWN_TIMEOUT` (default 20s) and 5s `workerDrainGrace`. Asynq own shutdown cannot beat its `TaskCheckInterval` on idle queue (`processor.go` sleep uninterruptibly for roughly half of it), so 15s of check interval = floor on worker drain — that is what grace window absorb.

Support package: `internal/extract` (deterministic CSS-selector schema extraction, extractive summary, PII redaction — all LLM-free), `internal/image` (srcset/lazy-load/`<picture>` extraction, dimension sniffing, resize/re-encode), `internal/sitemap` (robots.txt/sitemap.xml discovery behind `/v1/map`), `internal/search` (Brave), `internal/safeurl` (SSRF guard), `pkg/logger` (slog wrapper).

**Search backends** (`internal/search`): chain **Native → SearXNG → Stealth → Brave** (`NewHybridServiceWithNative`). Native = declarative roster in `internal/search/engines/registry.default.yaml` (override `SEARCH_ENGINES_PATH`) fanned out per category (`general|news`, API `code` → registry `it`) with a per-engine `Report` that feeds `unresponsive_engines`. SearXNG JSON contract listener: `SEARCH_COMPAT_ADDR` (use `:7436` locally — `:7435` is the SearXNG container). Proxy groups `webshare|tier1|tier2` via `WEBSHARE_API_KEY` (list fetch, lazy) or `WEBSHARE_PROXY_URL` (explicit, wins) + `SEARCH_PROXY_TIER1/T2`, byte cap `SEARCH_PROXY_BUDGET_GB` (default 1, 0=unlimited). Live canary: `go test -tags=canary ./internal/search/engines/ -run TestCanary -v` (`CANARY_ENGINES=ddg,bing` filters; never in CI). **Roster changes require a live probe first** — verdicts logged in local-only `plan/tomoshi-search/research.md` §3b (e.g. mojeek 403 from datacenter egress, Google News links JS-only, reuters feeds dead).

## 📝 Development Conventions

- Log through `pkg/logger` (`logger.Log`). No `fmt.Println`, no raw `log`. Package whose test exercise real callback need `TestMain` calling `logger.Init("error")` or `logger.Log` is nil and they panic.
- Handle every error explicit; wrap with `fmt.Errorf("...: %w", err)`. No `_ = err`, no `panic` for control flow, no start goroutine without lifecycle.
- Propagate `context.Context` into anything blocking or networked.
- Table-driven test with `t.Run` subtest; mock via `domain.Scraper` / `search.Service` interface (see `MockSearchService`, `mockScraper`). Handler test use `httptest` with `gin.SetMode(gin.TestMode)`.
- User agent come from `gofakeit.UserAgent()` — no hardcode one (exception: search engines use the fixed profile-matched Chrome UA in `internal/search/engines/fetch.go` — a mismatched/random UA is itself a bot signal there).
- New config go in `internal/config/config.go` (Viper + godotenv, `AutomaticEnv` with `.`→`_`), plus `.env.example` and README table.
- **Interfaces:** Define interfaces in `internal/domain` to keep the core logic decoupled from implementation details.
- **Swagger:** API documentation is auto-generated using `swag`. In debug mode, the API attempts to re-generate docs on startup.
- **Concurrency:** The worker is configured for 10 concurrent jobs by default (adjustable in `internal/worker/server.go`).

## ⚠️ Gotchas

- `.gitignore` used to end with a bare `*.json` that kept EVERY json file out of git (incl. `internal/api/docs/swagger.json`); patterns are now scoped (`/testdata-*.json`, `/scratch*.json`, `/*.local.json`) so testdata/fixtures CAN be tracked — before committing a JSON file, check it against those scoped patterns.
- In `debug` mode `cmd/api/main.go` shell out to `go run .../swag@latest init` on every startup to regenerate `internal/api/docs`. Need network on first run and only log on failure. Set `SERVER_MODE=release` to skip. Swagger UI served only in debug mode.
- Stale multi-hundred-MB binary (`api`, `app`, `cinder-api`, `worker`) sit at repo root. Untracked now (`git rm --cached`, and `.gitignore` name them) but still exist on disk and in every past commit — they not build output of current tree, so no rebuild into those path, no trust them. `scripts/purge-binaries-from-history.sh` remove them from history; deliberately not run for you, since it rewrite every SHA and need force-push.
- `/`, `/health`, and `/v1/ping` skip auth and rate limit on purpose (load balancer and uptime probe). Everything else under `/v1` go through `APIKeyAuth` + `RateLimit`, both no-op until `API_KEYS` / `RATE_LIMIT_RPM` set.
- Redis config resolve in precedence order: `REDIS_URL`, then `REDIS_HOST`/`PORT`/`PASSWORD`, then `UPSTASH_REDIS_REST_URL` + `UPSTASH_REDIS_REST_TOKEN` (derive into `rediss://` URL). `rediss://` get TLS with TLS 1.2 minimum in `worker.RedisClientOpt`.
- `ScrapePayload.Render` = deprecated bool that predate `Mode`; when true it override `Mode`. Prefer `Mode`.

## 📚 Docs

`README.md` = API reference for endpoint, parameter, env var. Only three deeper docs stay tracked: `docs/guides/API_REFERENCE.md` (linked from the published skill), `docs/guides/SEARXNG_FLY.md` (linked from `packages/api/.env.example`), `docs/SEARCH_COMPARISON.md` (linked from README + web homepage). The rest of `docs/` (guides, features, design plans) and all of `plan/` are LOCAL-ONLY — gitignored; they still exist on disk but are not part of the public repo. `test_reports/` are historical and gitignored.

## 🗺️ Roadmap Focus

- Increasing test coverage (currently low).
- Implementing "Smart Wait" heuristics for SPAs.
- Enhancing browser health monitoring to prevent memory leaks.
