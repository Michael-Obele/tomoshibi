<p align="center">
  <img src="./banner.svg" alt="Tomoshibi — lamp light banner with paper lantern and Japanese characters" width="100%" />
</p>

<p align="center">
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License"/></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25+"/></a>
  <a href="https://bun.sh"><img src="https://img.shields.io/badge/Bun-1.2%2B-f9f1e1?logo=bun" alt="Bun"/></a>
  <a href="https://svelte.dev"><img src="https://img.shields.io/badge/Svelte-5-ff3e00?logo=svelte&logoColor=white" alt="Svelte 5"/></a>
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/v/tomoshi?label=tomoshi&color=cb0000" alt="npm version"/></a>
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/dm/tomoshi" alt="npm downloads"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi/actions/workflows/npm.yml"><img src="https://github.com/Michael-Obele/tomoshibi/actions/workflows/npm.yml/badge.svg" alt="npm provenance"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi"><img src="https://img.shields.io/github/stars/Michael-Obele/tomoshibi?style=social" alt="GitHub stars"/></a>
</p>

<h1 align="center">Tomoshibi <span style="font-weight:400">灯火</span></h1>

<p align="center"><em>lamp light — self-hosted search &amp; scraping for AI agents</em><br/>Turn any site into LLM-ready markdown in 200ms. One binary, $5/mo. Formerly <a href="https://github.com/Michael-Obele/cinder">Cinder</a> — <code>tomoshi</code> CLI alias.</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#packages">Packages</a> ·
  <a href="#api">API</a> ·
  <a href="#mcp">MCP</a> ·
  <a href="#web">Web</a> ·
  <a href="#benchmarks">Benchmarks</a>
</p>

---

## Why Tomoshibi

If you're paying Firecrawl/Exa by the token or spawning a Playwright per request, you're overpaying.

| What hurts with hosted APIs             | What Tomoshibi gives you                            | Outcome                             |
| --------------------------------------- | --------------------------------------------------- | ----------------------------------- |
| **$0.01–0.10 per scrape** + rate limits | **$0 self-hosted** — one binary, hobby-tier RAM     | Ship RAG without a cloud bill       |
| **500ms Chrome spawn per request**      | **One shared allocator + recycled tabs**            | ~200ms static, parallel pools       |
| **JS SPAs return empty HTML**           | **Smart mode** — static first, fallback to Chromedp | Works on React/Vue without guessing |
| **Noisy HTML (nav/ads/footer)**         | **Readability main-content** + ad block             | Clean markdown your LLM wants       |
| **Crawl needs a worker fleet**          | **Monolith** — Gin + Asynq in one process           | Pay per container, not per service  |

**Proof:** SearXNG search benchmark (`scripts/search-bench.py`, 10 workers/30s) — **Tomoshibi 560 req/s p50 11ms** vs Firecrawl self-hosted **1.9 req/s p50 5.4s** — ~300× throughput, same $0. See [`docs/SEARCH_COMPARISON.md`](docs/SEARCH_COMPARISON.md).

---

## Quick start

### Full stack — Docker Compose (recommended)

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
docker compose up -d              # api (7431) + mcp (7433) + web (7432) + redis (7434) + searxng (7435)
curl http://localhost:7431/health   # → {"status":"ok","service":"tomoshibi"}
```

All 5 services on `7431`–`7435` (avoids clashing with 3000/8000/8080). Every feature works — scrape, crawl, batch, monitor, search.

### Individual services

```bash
# Pull from Docker Hub or GHCR — pick one image or all
# Docker Hub
docker pull michaelobele/tomoshibi-api:latest    # api only (7431)
docker pull michaelobele/tomoshibi-mcp:latest    # mcp only (7433)
docker pull michaelobele/tomoshibi-web:latest    # web only (7432)
docker pull michaelobele/tomoshibi:latest        # all-in-one
# GHCR (same images)
docker pull ghcr.io/michael-obele/tomoshibi-api:latest
docker pull ghcr.io/michael-obele/tomoshibi-mcp:latest
docker pull ghcr.io/michael-obele/tomoshibi-web:latest
docker pull ghcr.io/michael-obele/tomoshibi:latest

# Run one service standalone (needs a running API on 7431)
docker run --rm -p 7431:7431 michaelobele/tomoshibi-api
docker run --rm -p 7433:7433 -e TOMOSHIBI_API_URL=http://host.docker.internal:7431 michaelobele/tomoshibi-mcp
docker run --rm -p 7432:7432 -e TOMOSHIBI_API_URL=http://host.docker.internal:7431 michaelobele/tomoshibi-web

# Or via compose per-package
docker compose -f packages/api/docker-compose.yml up -d   # api + redis + searxng
docker compose -f packages/mcp/docker-compose.yml up -d   # mcp (needs api)
docker compose -f packages/web/docker-compose.yml up -d   # web (needs api)
```

### From source

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
go run ./packages/api/cmd/api              # needs Go 1.25+ and Chromium on PATH (7431)
REDIS_URL=redis://localhost:7434 go run ./packages/api/cmd/api   # with async
```

### Your first scrape

```bash
curl -X POST http://localhost:7431/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

```json
{
  "url": "https://example.com",
  "markdown": "# Example Domain\n\n...",
  "metadata": { "title": "Example Domain" }
}
```

No `mode` needed — `smart` is default. `static` skips JS; `dynamic` always renders.

---

## Packages

| Package | Path                           | Description                                                                                    | Install                                                |
| ------- | ------------------------------ | ---------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| **api** | [`packages/api`](packages/api) | Go scraping API — Gin + Chromedp + Colly + SearXNG                                             | `docker compose up` or `go run ./packages/api/cmd/api` |
| **mcp** | [`packages/mcp`](packages/mcp) | MCP server — 3 tools, `tomoshi`/`tomoshibi` bin ([npm](https://www.npmjs.com/package/tomoshi)) | `npx -y tomoshi`                                       |
| **web** | [`packages/web`](packages/web) | Svelte 5 playground — scrape/crawl/search UI                                                   | `bun --cwd packages/web dev`                           |

```bash
# All packages
bun install          # root (if using workspaces)
# Or per-package
bun --cwd packages/mcp install && bun --cwd packages/mcp dev
bun --cwd packages/web install && bun --cwd packages/web dev
```

---

## API

Base: `http://localhost:7431` — every endpoint under `/v1`.

| Method | Endpoint              | What it does                                                                 | Needs Redis |
| ------ | --------------------- | ---------------------------------------------------------------------------- | ----------- |
| `POST` | `/v1/scrape`          | Single page → markdown (smart/static/dynamic, screenshots, `extract_schema`) | No          |
| `POST` | `/v1/scrape` + `urls` | Multi-URL sync (max 10, no Redis)                                            | No          |
| `POST` | `/v1/map`             | Sitemap → robots.txt → link fallback                                         | No          |
| `POST` | `/v1/search`          | SearXNG aggregated search (Brave fallback)                                   | No          |
| `POST` | `/v1/crawl`           | Async BFS crawl → `202` + job ID                                             | **Yes**     |
| `GET`  | `/v1/crawl/:id`       | Poll crawl status                                                            | **Yes**     |
| `POST` | `/v1/batch/scrape`    | Enqueue 20 URLs async                                                        | **Yes**     |
| `GET`  | `/v1/batch/:id`       | Poll batch                                                                   | **Yes**     |
| `POST` | `/v1/monitor`         | Hash markdown, webhook on change                                             | **Yes**     |

```
Client → Gin Router → Scraper (Colly / Chromedp + Readability)
                 ↘ Asynq (Redis) → Worker → same Scraper
                                         ↘ Browser Pool (one allocator, recycled every 100 scrapes)
```

---

## MCP

3 tools, not 8 — resource-oriented (`action` enum, ≤7 philosophy).

| Tool               | `action`                                                       | Endpoint                             |
| ------------------ | -------------------------------------------------------------- | ------------------------------------ |
| `tomoshi_extract`  | `scrape` · `scrape_multi` · `links` · `batch` · `batch_status` | `/v1/scrape`, `/v1/batch`            |
| `tomoshi_discover` | `search` · `map` · `crawl` · `crawl_status`                    | `/v1/search`, `/v1/map`, `/v1/crawl` |
| `tomoshi_monitor`  | `create` · `status` · `delete`                                 | `/v1/monitor`                        |

```jsonc
// .vscode/mcp.json or Claude config
{
  "mcpServers": {
    "tomoshi": {
      "command": "npx",
      "args": ["-y", "tomoshi"],
      "env": { "TOMOSHIBI_API_URL": "http://localhost:7431" },
    },
  },
}
```

Also: `npx tomoshibi` (alias, same package).

---

## Web

Svelte 5 playground for scrape/crawl/search — same API, browser UI.

```bash
bun --cwd packages/web dev   # http://localhost:5173 (dev) / 7432 (docker)
```

Remote Functions proxy to the Go backend; API keys stay server-side.

---

## Benchmarks

| Stack         | Pull size | Running RAM | Containers |
| ------------- | --------- | ----------- | ---------- |
| **Tomoshibi** | ~1.79 GB  | ~317 MiB    | 4          |
| Firecrawl     | ~7.26 GB  | ~4.1 GiB    | 6          |

Measured 2026-08-31 (`docker images` + `docker stats`). **4× lighter, 13× less RAM.** Fits on 512 MB Fly.io / $5 Hetzner — Firecrawl doesn't.

---

## Name

**Tomoshibi** (灯火 — ともしび) — Japanese for _lamp light_, the glow of a paper lantern. Search as illumination: lighting up the web for AI agents. Short alias **tomoshi** (灯) for daily use. Formerly **Cinder** (extinct fire — wrong metaphor for discovery; `cinder` also collided with `github.com/cinder/Cinder` 5.5k★ and `npm/cinder`).

---

## License

MIT — see [LICENSE](LICENSE).

> **Migrating from Cinder?** `github.com/Michael-Obele/cinder` now redirects to `tomoshibi`. Docker image `ghcr.io/michael-obele/cinder` → `ghcr.io/michael-obele/tomoshibi`. No code changes needed beyond the import path (`github.com/Michael-Obele/tomoshibi`).
