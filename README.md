<p align="center">
  <img src="./banner.svg" alt="Tomoshibi — lamp light banner with paper lantern and Japanese characters" width="100%" />
</p>

<p align="center">
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License"/></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25+"/></a>
  <a href="https://bun.sh"><img src="https://img.shields.io/badge/Bun-1.2%2B-f9f1e1?logo=bun" alt="Bun"/></a>
  <a href="https://svelte.dev"><img src="https://img.shields.io/badge/Svelte-5-ff3e00?logo=svelte&logoColor=white" alt="Svelte 5"/></a>
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

| What hurts with hosted APIs | What Tomoshibi gives you | Outcome |
|---|---|---|
| **$0.01–0.10 per scrape** + rate limits | **$0 self-hosted** — one binary, hobby-tier RAM | Ship RAG without a cloud bill |
| **500ms Chrome spawn per request** | **One shared allocator + recycled tabs** | ~200ms static, parallel pools |
| **JS SPAs return empty HTML** | **Smart mode** — static first, fallback to Chromedp | Works on React/Vue without guessing |
| **Noisy HTML (nav/ads/footer)** | **Readability main-content** + ad block | Clean markdown your LLM wants |
| **Crawl needs a worker fleet** | **Monolith** — Gin + Asynq in one process | Pay per container, not per service |

**Proof:** SearXNG search benchmark (`scripts/search-bench.py`, 10 workers/30s) — **Tomoshibi 560 req/s p50 11ms** vs Firecrawl self-hosted **1.9 req/s p50 5.4s** — ~300× throughput, same $0. See [`docs/SEARCH_COMPARISON.md`](docs/SEARCH_COMPARISON.md).

---

## Quick start

### Full stack — Docker Compose (recommended)

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
docker compose up -d          # api + redis + searxng
curl http://localhost:8080/health   # → {"status":"ok","service":"tomoshibi"}
```

SearXNG on `http://localhost:8889`. Every feature works — scrape, crawl, batch, monitor, search.

### Single container

```bash
docker build -t tomoshibi -f packages/api/Dockerfile packages/api
docker run --rm -p 8080:8080 -e SERVER_MODE=release tomoshibi
# published: ghcr.io/michael-obele/tomoshibi
```

### From source

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
go run ./packages/api/cmd/api              # needs Go 1.25+ and Chromium on PATH
REDIS_URL=redis://localhost:6379 go run ./packages/api/cmd/api   # with async
```

### Your first scrape

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

```json
{ "url": "https://example.com", "markdown": "# Example Domain\n\n...", "metadata": { "title": "Example Domain" } }
```

No `mode` needed — `smart` is default. `static` skips JS; `dynamic` always renders.

---

## Packages

| Package | Path | Description | Install |
|---|---|---|---|
| **api** | [`packages/api`](packages/api) | Go scraping API — Gin + Chromedp + Colly + SearXNG | `docker compose up` or `go run ./packages/api/cmd/api` |
| **mcp** | [`packages/mcp`](packages/mcp) | MCP server for Claude/Cursor/Zed — 3 tools, `tomoshibi`/`tomoshi` bin | `npx tomoshibi` or `bunx tomoshibi` |
| **web** | [`packages/web`](packages/web) | Svelte 5 playground — scrape/crawl/search UI | `bun --cwd packages/web dev` |

```bash
# All packages
bun install          # root (if using workspaces)
# Or per-package
bun --cwd packages/mcp install && bun --cwd packages/mcp dev
bun --cwd packages/web install && bun --cwd packages/web dev
```

---

## API

Base: `http://localhost:8080` — every endpoint under `/v1`.

| Method | Endpoint | What it does | Needs Redis |
|---|---|---|---|
| `POST` | `/v1/scrape` | Single page → markdown (smart/static/dynamic, screenshots, `extract_schema`) | No |
| `POST` | `/v1/scrape` + `urls` | Multi-URL sync (max 10, no Redis) | No |
| `POST` | `/v1/map` | Sitemap → robots.txt → link fallback | No |
| `POST` | `/v1/search` | SearXNG aggregated search (Brave fallback) | No |
| `POST` | `/v1/crawl` | Async BFS crawl → `202` + job ID | **Yes** |
| `GET` | `/v1/crawl/:id` | Poll crawl status | **Yes** |
| `POST` | `/v1/batch/scrape` | Enqueue 20 URLs async | **Yes** |
| `GET` | `/v1/batch/:id` | Poll batch | **Yes** |
| `POST` | `/v1/monitor` | Hash markdown, webhook on change | **Yes** |

```
Client → Gin Router → Scraper (Colly / Chromedp + Readability)
                 ↘ Asynq (Redis) → Worker → same Scraper
                                         ↘ Browser Pool (one allocator, recycled every 100 scrapes)
```

---

## MCP

3 tools, not 8 — resource-oriented (`action` enum, ≤7 philosophy).

| Tool | `action` | Endpoint |
|---|---|---|
| `tomoshibi_extract` | `scrape` · `scrape_multi` · `links` · `batch` · `batch_status` | `/v1/scrape`, `/v1/batch` |
| `tomoshibi_discover` | `search` · `map` · `crawl` · `crawl_status` | `/v1/search`, `/v1/map`, `/v1/crawl` |
| `tomoshibi_monitor` | `create` · `status` · `delete` | `/v1/monitor` |

```jsonc
// .vscode/mcp.json or Claude config
{
  "mcpServers": {
    "tomoshibi": {
      "command": "npx",
      "args": ["-y", "tomoshibi"],
      "env": { "TOMOSHIBI_API_URL": "http://localhost:8080" }
    }
  }
}
```

Also: `npx tomoshi` (short alias, same package).

---

## Web

Svelte 5 playground for scrape/crawl/search — same API, browser UI.

```bash
bun --cwd packages/web dev   # http://localhost:5173
```

Remote Functions proxy to the Go backend; API keys stay server-side.

---

## Benchmarks

| Stack | Pull size | Running RAM | Containers |
|---|---|---|---|
| **Tomoshibi** | ~1.79 GB | ~317 MiB | 4 |
| Firecrawl | ~7.26 GB | ~4.1 GiB | 6 |

Measured 2026-08-31 (`docker images` + `docker stats`). **4× lighter, 13× less RAM.** Fits on 512 MB Fly.io / $5 Hetzner — Firecrawl doesn't.

---

## Name

**Tomoshibi** (灯火 — ともしび) — Japanese for *lamp light*, the glow of a paper lantern. Search as illumination: lighting up the web for AI agents. Short alias **tomoshi** (灯) for daily use. Formerly **Cinder** (extinct fire — wrong metaphor for discovery; `cinder` also collided with `github.com/cinder/Cinder` 5.5k★ and `npm/cinder`).

---

## License

MIT — see [LICENSE](LICENSE).

> **Migrating from Cinder?** `github.com/Michael-Obele/cinder` now redirects to `tomoshibi`. Docker image `ghcr.io/michael-obele/cinder` → `ghcr.io/michael-obele/tomoshibi`. No code changes needed beyond the import path (`github.com/Michael-Obele/tomoshibi`).

