<p align="center">
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25+"/></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License"/></a>
  <a href="https://hub.docker.com/r/michaelobele/tomoshibi-api"><img src="https://img.shields.io/docker/v/michaelobele/tomoshibi-api?label=docker" alt="Docker image"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi"><img src="https://img.shields.io/github/stars/Michael-Obele/tomoshibi?style=social" alt="GitHub stars"/></a>
</p>

# Tomoshibi API _(packages/api)_

Go scraping API — Gin HTTP server + embedded Asynq worker in one binary.

The Go backend of [**Tomoshibi 灯火**](../../README.md) — a self-hosted, Firecrawl-free web scraping API that turns any site into LLM-ready markdown.

One binary runs everything: the **Gin HTTP server** plus the **embedded Asynq worker** (crawl, batch, monitor). No separate worker process to deploy.

- **Smart scraping** — Colly (static) first, Chromedp (JS rendering) only when the page needs it
- **Search** — in-house 21-engine roster (keyless + keyed `brave`/`serper`/`tavily`) → SearXNG → stealth → Brave fallback
- **Async jobs** — crawl / batch / monitor through Redis (optional; sync endpoints work with no Redis at all)
- **Hobby-tier sized** — fits a 512 MB VM; Docker image ships Chromium included

> All commands below run from `packages/api` unless noted.

**Contents:** [Quick start](#quick-start) · [Project structure](#project-structure) · [Endpoints](#endpoints) · [Configuration](#configuration) · [Development](#development) · [Docker image](#docker-image) · [Deploy](#deploy) · [Troubleshooting](#troubleshooting) · [Related](#related)

---

## Quick start

### Option A — Docker Compose (recommended)

Starts the API + Redis + SearXNG together:

```bash
# from the repo root
docker compose -f packages/api/docker-compose.yml up -d

# or from this directory
cd packages/api && docker compose up -d
```

| Service | Port   | Purpose                         |
| ------- | ------ | ------------------------------- |
| api     | `7431` | HTTP API                        |
| redis   | `7434` | async crawl/batch/monitor queue |
| searxng | `7435` | self-hosted search backend      |

Verify:

```bash
curl http://localhost:7431/health
# → {"status":"ok","service":"tomoshibi"}

curl -X POST http://localhost:7431/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

> The image build downloads a Go toolchain, Alpine packages and Chromium — roughly 1–2 GB total on first build. Later builds are cached.

### Option B — From source

**Prerequisites:** Go 1.25+, and Chromium for `dynamic`/`smart` rendering (see [Troubleshooting](#troubleshooting) if you don't have one).

```bash
cd packages/api
go mod download
cp .env.example .env    # optional — defaults are enough for a first try
go run ./cmd/api        # → http://localhost:7431
```

Without Chromium the API still starts: `static` scraping works, `dynamic` degrades with a startup warning instead of failing later.

### Option C — Docker Hub (no build)

```bash
docker run --rm -p 7431:7431 michaelobele/tomoshibi-api:latest
```

### What needs Redis?

| Endpoint                                    | Without Redis          |
| ------------------------------------------- | ---------------------- |
| `POST /v1/scrape`, `/v1/map`, `/v1/search`  | ✅ work                |
| `POST /v1/crawl`                            | `503`                  |
| `POST /v1/batch/scrape`, `POST /v1/monitor` | not registered → `404` |

To run API-only from source: `DISABLE_WORKER=true go run ./cmd/api`. A standalone worker (`go run ./cmd/worker`) exists for split deployments — don't run it next to the monolith, or the same queue gets consumed twice.

---

## Project structure

```text
packages/api/
├── cmd/                      # entry points (package main)
│   ├── api/                  # monolith — Gin HTTP server + embedded Asynq worker
│   ├── worker/               # standalone Asynq worker (split deployments)
│   ├── debug_asynq/          # queue inspector — dump/inspect Asynq tasks in Redis
│   ├── demo-integration/     # scripted client that exercises the /v1 endpoints
│   └── e2e_test/             # end-to-end smoke run against a live API on :7431
├── internal/                 # private packages — import only domain/ and pkg/
│   ├── api/                  # HTTP layer
│   │   ├── router.go         # route table, auth + rate-limit wiring, probes
│   │   ├── handlers/         # scrape · search · map · crawl · batch · monitor
│   │   ├── middleware/       # APIKeyAuth · RateLimit · Logger
│   │   ├── docs/             # generated Swagger (swag init / debug startup)
│   │   └── router_test.go    # route + probe tests
│   ├── scraper/              # scraping core — Colly, Chromedp, smart-mode heuristics
│   ├── search/               # search chain: Native → SearXNG → Stealth → Brave
│   │   ├── engines/          # declarative YAML roster, fetch, proxy transport
│   │   └── compat/           # SearXNG JSON contract listener + golden fixtures
│   ├── worker/               # Asynq task definitions + server setup
│   ├── domain/               # core types & interfaces — the mocking seam
│   ├── config/               # Viper + godotenv configuration
│   ├── extract/              # schema extraction, extractive summary, PII redaction
│   ├── image/                # srcset/lazy-load extraction, resize, re-encode
│   ├── sitemap/              # robots.txt / sitemap.xml discovery → /v1/map
│   └── safeurl/              # SSRF guard (post-DNS dial hook + pre-flight check)
├── pkg/logger/               # slog wrapper (logger.Log) — never fmt.Println
├── test/                     # integration tests — httptest, externals mocked
├── scripts/                  # search-bench.py, load-harness.go, fly-searxng.sh
├── deploy/                   # searxng/ configs for local compose only
├── Dockerfile                # multi-stage: Go build → Alpine + Chromium + tini
├── docker-compose.yml        # api (7431) + redis (7434) + searxng (7435)
├── Makefile                  # make check · fmt · vet · staticcheck · lint · test
├── fly.toml / render.yaml    # deploy targets (Fly.io 512 MB, Render)
├── install_browser.sh        # install Chromium for local dynamic scraping
├── .env.example              # every env var, documented inline
└── README.md                 # this file
```

Dependency rule: `cmd/` → `internal/api` → `internal/scraper` → `internal/domain`; `internal/*` may only import `domain` or `pkg/`.

---

## Endpoints

Base URL `http://localhost:7431` — everything lives under `/v1`.

| Method | Endpoint                   | What it does                                                                              | Redis   |
| ------ | -------------------------- | ----------------------------------------------------------------------------------------- | ------- |
| `POST` | `/v1/scrape`               | single page → markdown (modes: `smart`/`static`/`dynamic`, screenshots, `extract_schema`) | No      |
| `POST` | `/v1/scrape` + `urls`      | multi-URL sync (max 10)                                                                   | No      |
| `POST` | `/v1/map`                  | sitemap → robots.txt → link discovery                                                     | No      |
| `POST` | `/v1/search`               | aggregated search (Native → SearXNG → Stealth → Brave)                                    | No      |
| `GET`  | `/v1/env`                  | env var names a client may supply per request in `X-Tomoshi-Env` (see Search key forwarding below) | No |
| `POST` | `/v1/crawl`                | async BFS crawl → `202` + job ID                                                          | **Yes** |
| `GET`  | `/v1/crawl/:id`            | poll crawl status                                                                         | **Yes** |
| `POST` | `/v1/batch/scrape`         | enqueue up to 20 URLs                                                                     | **Yes** |
| `GET`  | `/v1/batch/:id`            | poll batch                                                                                | **Yes** |
| `POST` | `/v1/monitor`              | hash markdown, webhook on change                                                          | **Yes** |
| `GET`  | `/`, `/health`, `/v1/ping` | unauthenticated liveness probes (on purpose)                                              | No      |
| `GET`  | `/swagger/*`               | Swagger UI — **debug mode only**                                                          | No      |

Full request/response reference: [`docs/guides/API_REFERENCE.md`](../../docs/guides/API_REFERENCE.md).

### Search key forwarding

A client may supply allowlisted search API keys **per request** in the `X-Tomoshi-Env` header (JSON object of name → value, e.g. `{"BRAVE_SEARCH_API_KEY":"BSB-..."}`). The middleware (`internal/api/middleware/request_env.go`) validates against the allowlist in `internal/search/envctx` and attaches the values to the request context; engines resolve `requires_env` and `${VAR}` through `envctx.Get`, so overrides apply to **that request only** — never process-global, never stored. `GET /v1/env` returns the accepted names so clients can discover them (the MCP forwards matching keys from its own env via `TOMOSHI_FORWARD_ENV`).

### Scrape modes

| Mode              | Engine                                                            | Use when                                       |
| ----------------- | ----------------------------------------------------------------- | ---------------------------------------------- |
| `smart` (default) | Colly first → Chromedp if heuristics say the HTML is an SPA shell | you don't know or don't care                   |
| `static`          | Colly only                                                        | fast, plain pages                              |
| `dynamic`         | Chromedp always                                                   | JS-rendered apps, page `actions`, `screenshot` |

`static` + `actions` is a hard error — actions need a real browser.

---

## Configuration

Copy `.env.example` to `.env` (loaded via Viper + godotenv, `AutomaticEnv`). Every key also works as a plain environment variable, so `docker run -e ...` and `fly secrets set ...` behave the same.

| Variable                                  | Default                 | Notes                                                                                                            |
| ----------------------------------------- | ----------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `SERVER_PORT`                             | `7431`                  | Fly injects `8080` (also accepts `PORT`)                                                                         |
| `SERVER_MODE`                             | `debug`                 | `debug` = Swagger UI + `swag` regen on startup; `release` skips it                                               |
| `REDIS_URL`                               | —                       | needed for crawl/batch/monitor; `rediss://`, `REDIS_HOST`/`PORT`/`PASSWORD` and Upstash REST vars also supported |
| `SEARXNG_ENDPOINT`                        | `http://localhost:7435` | second step of the search chain                                                                                  |
| `BRAVE_SEARCH_API_KEY`                    | —                       | fallback search (free $5/mo credit ≈ 1,000 searches) — or skip server config entirely: a client may send it per request in `X-Tomoshi-Env` |
| `SERPER_API_KEY`                          | —                       | optional keyed engine `serper` (Google results; 2,500 free queries, no card)                                                     |
| `TAVILY_API_KEY`                          | —                       | optional keyed engine `tavily` (1,000 free credits/mo)                                                                           |
| `SEARCH_NATIVE_ENABLED`                   | `true`                  | in-house engine roster (`internal/search/engines/registry.default.yaml`)                                         |
| `SEARCH_COMPAT_ADDR`                      | —                       | e.g. `:7435` to expose a SearXNG-compatible JSON API                                                             |
| `WEBSHARE_API_KEY` / `WEBSHARE_PROXY_URL` | —                       | proxy pool for the search engines                                                                                |
| `APP_API_KEYS`                            | —                       | empty = no auth; comma-separated keys (alias: `API_KEYS`)                                                        |
| `APP_RATE_LIMIT_RPM`                      | `0`                     | `0` = unlimited (alias: `RATE_LIMIT_RPM`)                                                                        |
| `DISABLE_WORKER`                          | `false`                 | `true` = HTTP server only                                                                                        |
| `SSRF_ALLOW_PRIVATE`                      | `false`                 | `true` only for scraping internal hosts                                                                          |

Tuning knobs (`CHROME_RECYCLE_AFTER`, `CRAWL_CONCURRENCY`, `SHUTDOWN_TIMEOUT`, …) are documented inline in [`.env.example`](.env.example) — that file is the source of truth.

> Auth and rate limiting are no-ops until `APP_API_KEYS` / `APP_RATE_LIMIT_RPM` are set. `/`, `/health` and `/v1/ping` always skip both (load balancer and uptime probes).

### Search backends

Requests fall through this chain, first success wins:

```text
Native engines → SearXNG → Stealth → Brave
```

- **Native** — declarative YAML roster, per-category fan-out (`general`/`news`, API `code` → registry `it`), reports dead engines into `unresponsive_engines`
- **SearXNG** — the container in `docker-compose.yml`
- **Stealth** — reuses Chromedp when both upstreams fail (`STEALTH_ENABLED=true`)
- **Brave** — API fallback

Roster changes need a live probe first:

```bash
go test -tags=canary ./internal/search/engines/ -run TestCanary -v   # CANARY_ENGINES=ddg,bing to filter
```

Never run the canary in CI. SearXNG runs locally only (`docker compose up -d searxng`); Fly ships no sidecar, the native roster and keyed engines cover search there.

---

## Development

```bash
make check        # fmt + vet + staticcheck + test — run this before finishing work
make fmt          # go fmt ./...
make vet          # go vet ./...
make staticcheck  # requires: go install honnef.co/go/tools/cmd/staticcheck@latest
make lint         # requires golangci-lint
make test         # go test -race -count=1 -v ./...
```

Targeted runs:

```bash
go test ./internal/scraper/... -v                            # one package
go test ./internal/scraper/... -run TestShouldUseDynamic      # one test
go test ./test/... -v                                        # integration (mocks only)
go test ./internal/... ./pkg/... -coverprofile=coverage.out && go tool cover -func=coverage.out
```

Tests need no Redis and no Brave key — externals are mocked. Chromedp tests need a local Chromium. Package tests that exercise real log callbacks need `TestMain` calling `logger.Init("error")`.

**Swagger:** update handlers/structs, then run `swag init` (debug mode also regenerates on startup).

**Benchmarks:** `scripts/search-bench.py` (the numbers in the [search comparison](../../docs/SEARCH_COMPARISON.md)) and `scripts/load-harness.go`.

Conventions (error wrapping, context propagation, table-driven tests, `gofakeit` user agents) are written down in [`AGENTS.md`](../../AGENTS.md). The full annotated tree lives in [Project structure](#project-structure).

---

## Docker image

Multi-stage build ([`Dockerfile`](Dockerfile)):

1. `golang:1.25-alpine` compiles `tomoshibi-api` and `tomoshibi-worker` (static, `CGO_ENABLED=0`)
2. `alpine:3.20` adds Chromium + fonts + `tini` (reaps Chrome's zombies) and sets `CHROME_BIN`/`CHROME_PATH`

```bash
docker build -t tomoshibi-api .
docker run --rm -p 7431:7431 \
  -e SERVER_MODE=release \
  -e REDIS_URL=redis://host.docker.internal:7434 \
  tomoshibi-api
```

Full stack from the repo root (`docker compose up -d`) also brings up Redis and SearXNG.

## Deploy

**Fly.io** — [`fly.toml`](fly.toml) (512 MB shared VM, `internal_port` 8080, `kill_timeout` 25s, health check on `/health`):

```bash
fly deploy
fly secrets set REDIS_URL=rediss://... BRAVE_SEARCH_API_KEY=... APP_API_KEYS=$(openssl rand -hex 32)
```

Keep `SHUTDOWN_TIMEOUT` (default 20s) below `kill_timeout` so in-flight requests drain before the VM dies.

**Render** — [`render.yaml`](render.yaml).

---

## Troubleshooting

| Symptom                                               | Fix                                                                                                                                                          |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `dynamic` mode fails / startup warning about Chromium | install a browser: `./install_browser.sh`, or `apt install chromium` / `brew install --cask chromium`. `CHROME_BIN` / `CHROME_PATH` override auto-detection. |
| `/v1/crawl` → `503`                                   | Redis isn't reachable — start it (`docker compose up -d redis`) or set `REDIS_URL`                                                                           |
| `/v1/batch/*`, `/v1/monitor` → `404`                  | same as above: those routes register only when Redis is configured                                                                                           |
| Swagger UI missing                                    | only served in `SERVER_MODE=debug`                                                                                                                           |
| `static` returns empty content on a React/Vue page    | expected — use `smart` (default) or `dynamic`; the SPA-shell heuristic triggers the Chromedp fallback                                                        |
| Outbound fetch refused as private/loopback            | SSRF guard — set `SSRF_ALLOW_PRIVATE=true` only for internal targets                                                                                         |
| Port already in use                                   | `SERVER_PORT=7432 go run ./cmd/api`                                                                                                                          |

---

## Related

- [Repo README](../../README.md) — project overview, benchmarks, full-stack quick start
- [API reference](../../docs/guides/API_REFERENCE.md) — every endpoint and parameter
- [Search comparison](../../docs/SEARCH_COMPARISON.md) — benchmark vs Firecrawl
- [MCP package](../mcp/README.md) · [Web playground](../web/README.md)

## License

MIT — see [LICENSE](../../LICENSE).
