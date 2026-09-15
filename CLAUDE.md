# CLAUDE.md

This file give guidance to Claude Code (claude.ai/code) for work with code in this repo.

## Project

Tomoshibi = self-hosted web scrape API in Go (formerly Cinder). Turn website into LLM-ready markdown (open-source Firecrawl alternative). Ship as **monolith with embedded worker**: `cmd/api` run both Gin HTTP server and Asynq background worker in one process, sized for 512MB–1GB hobby-tier host.

## Commands

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

Docker: `docker build -t cinder . && docker run -p 8080:8080 -e SERVER_MODE=release cinder`. `docker-compose.yml` bring up Redis too. Deploy target = Fly.io via `fly.toml`.

Test need no Redis, no Brave key — all external thing mocked. Chromedp test need local Chromium.

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

## Conventions

- Log through `pkg/logger` (`logger.Log`). No `fmt.Println`, no raw `log`. Package whose test exercise real callback need `TestMain` calling `logger.Init("error")` or `logger.Log` is nil and they panic.
- Handle every error explicit; wrap with `fmt.Errorf("...: %w", err)`. No `_ = err`, no `panic` for control flow, no start goroutine without lifecycle.
- Propagate `context.Context` into anything blocking or networked.
- Table-driven test with `t.Run` subtest; mock via `domain.Scraper` / `search.Service` interface (see `MockSearchService`, `mockScraper`). Handler test use `httptest` with `gin.SetMode(gin.TestMode)`.
- User agent come from `gofakeit.UserAgent()` — no hardcode one.
- New config go in `internal/config/config.go` (Viper + godotenv, `AutomaticEnv` with `.`→`_`), plus `.env.example` and README table.

## Gotchas

- `.gitignore` end with `*.json`, so **no JSON file in repo is tracked** — including `internal/api/docs/swagger.json`. If task need committed JSON file, it need explicit negation or `git add -f`.
- In `debug` mode `cmd/api/main.go` shell out to `go run .../swag@latest init` on every startup to regenerate `internal/api/docs`. Need network on first run and only log on failure. Set `SERVER_MODE=release` to skip. Swagger UI served only in debug mode.
- Stale multi-hundred-MB binary (`api`, `app`, `cinder-api`, `worker`) sit at repo root. Untracked now (`git rm --cached`, and `.gitignore` name them) but still exist on disk and in every past commit — they not build output of current tree, so no rebuild into those path, no trust them. `scripts/purge-binaries-from-history.sh` remove them from history; deliberately not run for you, since it rewrite every SHA and need force-push.
- `/`, `/health`, and `/v1/ping` skip auth and rate limit on purpose (load balancer and uptime probe). Everything else under `/v1` go through `APIKeyAuth` + `RateLimit`, both no-op until `API_KEYS` / `RATE_LIMIT_RPM` set.
- Redis config resolve in precedence order: `REDIS_URL`, then `REDIS_HOST`/`PORT`/`PASSWORD`, then `UPSTASH_REDIS_REST_URL` + `UPSTASH_REDIS_REST_TOKEN` (derive into `rediss://` URL). `rediss://` get TLS with TLS 1.2 minimum in `worker.RedisClientOpt`.
- `ScrapePayload.Render` = deprecated bool that predate `Mode`; when true it override `Mode`. Prefer `Mode`.

## Docs

`README.md` = API reference for endpoint, parameter, env var. `docs/guides/` hold deeper guide (`ARCHITECTURE.md`, `API_REFERENCE.md`, `TESTING.md`, `CODE_WALKTHROUGH.md`, plus Go-for-Svelte-devs onboarding). `docs/features/` document each v2 feature. `plan/architecture.md` carry original design rationale. `cinder-tmcp/` (TMCP, not Mastra) is the MCP server — see `cinder-tmcp/README.md`. `test_reports/` are historical.
