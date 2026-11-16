<p align="center">
  <img src="./banner.svg" alt="Tomoshibi — lamp light banner with paper lantern and Japanese characters" width="100%" />
</p>

<p align="center">
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License"/></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25+"/></a>
  <a href="https://bun.sh"><img src="https://img.shields.io/badge/Bun-1.4%2B-f9f1e1?logo=bun" alt="Bun"/></a>
  <a href="https://svelte.dev"><img src="https://img.shields.io/badge/Svelte-5-ff3e00?logo=svelte&logoColor=white" alt="Svelte 5"/></a>
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/v/tomoshi?label=tomoshi&color=cb0000" alt="npm version"/></a>
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/dm/tomoshi" alt="npm downloads"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi/actions/workflows/ci.yml"><img src="https://github.com/Michael-Obele/tomoshibi/actions/workflows/ci.yml/badge.svg" alt="CI"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi"><img src="https://img.shields.io/github/stars/Michael-Obele/tomoshibi?style=social" alt="GitHub stars"/></a>
</p>

<h1 align="center">Tomoshibi <span style="font-weight:400">灯火</span></h1>

<p align="center"><em>lamp light — self-hosted search &amp; scraping for AI agents</em><br/>Turn any site into LLM-ready markdown in 200ms. One binary, $5/mo. Formerly <a href="https://github.com/Michael-Obele/cinder">Cinder</a> — <code>tomoshi</code> CLI alias.</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#compared-to-firecrawl-and-exa">Comparison</a> ·
  <a href="#packages">Packages</a> ·
  <a href="#api">API</a> ·
  <a href="#borrow-your-own-browser">Your browser</a> ·
  <a href="#agent-skills">Skills</a> ·
  <a href="./CONTRIBUTING.md">Contributing</a>
</p>

---

## Why Tomoshibi

If you're paying Firecrawl or Exa by the token, or spawning a Playwright per request, you're overpaying.

| What hurts with hosted APIs             | What Tomoshibi gives you                            | Outcome                             |
| --------------------------------------- | --------------------------------------------------- | ----------------------------------- |
| **$0.01–0.10 per scrape** + rate limits | **$0 self-hosted** — one binary, hobby-tier RAM     | Ship RAG without a cloud bill       |
| **500ms Chrome spawn per request**      | **One shared allocator + recycled tabs**            | ~200ms static, parallel pools       |
| **JS SPAs return empty HTML**           | **Smart mode** — static first, fallback to Chromedp | Works on React/Vue without guessing |
| **Sites that block datacenter traffic** | **Borrow your own browser** — no recipe needed      | Signed-in pages, real session       |
| **Crawl needs a worker fleet**          | **Monolith** — Gin + Asynq in one process           | Pay per container, not per service  |

## Compared to Firecrawl and Exa

Measured on one machine against live local instances (2026-08-30). Exa numbers are its published free-tier limits — its API is not load-testable here without a key.

**Search** — `scripts/search-bench.py`, 10 workers, 30s, 10 real queries:

| Metric | **Tomoshibi** | Firecrawl (self-hosted) | Exa (cloud) |
| --- | --- | --- | --- |
| Throughput | **560 req/s** | 1.9 req/s | rate-limited free tier |
| p50 latency | **11 ms** | 5.4 s | n/a (paid) |
| p95 latency | **21 ms** | 8.7 s | n/a (paid) |
| Success rate | **100%** (16,809/16,809) | 100% (60/60) | n/a |
| Cost | **$0** | $0 | paid API |

~**300× the throughput** and ~**500× lower p50** than self-hosted Firecrawl's search, at the same $0. Firecrawl's search is slow because it fetches and extracts each result page rather than returning result metadata.

**Scrape** — 5 concurrent `go.dev/blog`:

| Metric | **Tomoshibi** | Firecrawl (self-hosted) |
| --- | --- | --- |
| Latency (repeat) | **7–14 ms** (Redis-cached) | ~17 s each (no cache) |
| Cold latency | ~1–11 s (site-dependent) | ~17 s (always full Playwright) |
| JS rendering | ✅ Chromedp | ✅ Playwright |

**Footprint** (`docker images` + `docker stats`, 2026-08-31):

| Stack | Pull size | Running RAM | Containers |
| --- | --- | --- | --- |
| **Tomoshibi** | ~1.79 GB | ~317 MiB | 4 |
| Firecrawl | ~7.26 GB | ~4.1 GiB | 6 |

**4× lighter, 13× less RAM.** Fits on a 512 MB Fly.io box / $5 Hetzner — Firecrawl doesn't.

**Where Exa still wins:** semantic (vector) search and search highlights. Both are on the roadmap; everything else in the matrix below is here today.

<details>
<summary>Full feature matrix and how to reproduce the benchmarks</summary>

| Capability | **Tomoshibi** | Exa | Firecrawl (self-hosted) |
| --- | --- | --- | --- |
| Clean markdown fetch | ✅ | ✅ | ✅ |
| JS rendering (SPAs) | ✅ Chromedp | ❌ static | ✅ Playwright |
| Screenshots | ✅ | ❌ | ✅ |
| Image extraction (srcset/picture/lazy) | ✅ | ❌ | partial |
| Structured extraction (CSS selectors) | ✅ | ❌ | ✅ (AI + selector) |
| Extractive summary (LLM-free) | ✅ | ✅ | ❌ (LLM only) |
| PII redaction | ✅ | ❌ | ❌ |
| Page actions (click/wait/scroll) | ✅ | ❌ | ❌ |
| Signed-in / bot-protected pages | ✅ own browser | ❌ | ❌ |
| Multi-page crawl (BFS, globs, webhooks) | ✅ | partial | ✅ |
| Change-tracking monitors | ✅ | ❌ | ❌ |
| Semantic (vector) search | roadmap | ✅ | ❌ |
| Search highlights / category filters | roadmap | ✅ | ❌ |
| Self-hosted / no per-request cost | ✅ | ❌ paid | ✅ |

```bash
# search benchmark
python3 scripts/search-bench.py --url http://localhost:7431/v1/search \
  --concurrency 10 --duration 30 --label "Tomoshibi"

# scrape burst
for i in 1 2 3 4 5; do curl -s -o /dev/null -w "%{http_code} %{time_total}s\n" \
  -X POST http://localhost:7431/v1/scrape -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/blog","mode":"smart"}' & done; wait
```

Methodology, the Firecrawl side of every number, and the caveats: [`docs/SEARCH_COMPARISON.md`](docs/SEARCH_COMPARISON.md).

</details>

---

## Quick start

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
docker compose up -d                # api 7431 + redis 7434
curl http://localhost:7431/health   # → {"status":"ok","service":"tomoshibi"}
```

Ports `7431`–`7435` (avoids 3000/8000/8080). Scrape, crawl, batch, monitor and search all work out of the box; SearXNG is an opt-in profile.

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

### Other ways to run it

```bash
# from source (Go 1.25+ and Chromium on PATH)
go run ./packages/api/cmd/api
# with Redis for async work
REDIS_URL=redis://localhost:7434 go run ./packages/api/cmd/api

# published images
docker pull michaelobele/tomoshibi          # all-in-one
docker pull michaelobele/tomoshibi-api      # api only
docker pull michaelobele/tomoshibi-mcp      # mcp only
docker pull michaelobele/tomoshibi-web      # web only
# same images on GHCR: ghcr.io/michael-obele/tomoshibi-*
```

Per-package compose files, image tags and the build-arg gotcha for web: [`CONTRIBUTING.md`](./CONTRIBUTING.md#running-the-stack).

---

## Packages

Four packages, each with its own README. This file is the map; the detail lives next to the code.

| Package | What it is | Read |
| --- | --- | --- |
| **api** | Go scraping + search API. Gin, Colly, Chromedp, native engine roster (21 engines), embedded Asynq worker. Port 7431 | [**packages/api/README.md**](./packages/api/README.md) |
| **mcp** | MCP server — 3 resource-oriented tools over the API. npm [`tomoshi`](https://www.npmjs.com/package/tomoshi). Port 7433 | [**packages/mcp/README.md**](./packages/mcp/README.md) |
| **web** | Svelte 5 playground — scrape/crawl/search in a browser. Port 7432 | [**packages/web/README.md**](./packages/web/README.md) |
| **bridge** | Browser extension that lends Tomoshibi your own signed-in session. **PolyForm Noncommercial**, not MIT | [**packages/bridge/README.md**](./packages/bridge/README.md) |

---

## Documentation for agents

The API serves its own documentation, generated from the tracked markdown in this
repo — so the routes cannot describe a different API than the one answering.

| Route | What it is |
| --- | --- |
| `GET /llms.txt` | [llmstxt.org](https://llmstxt.org/) v2 index: summary, guidance, then links to every document |
| `GET /llms-full.txt` | Everything concatenated, for a large context window |
| `GET /docs` | Manifest as JSON: `version` (content digest), `api_version`, per-doc `sha256` and `source` |
| `GET /docs/<slug>.md` | One document as markdown |

Every response carries `X-Docs-Version` and an `ETag`, so an agent can ask
"anything new?" with one conditional request instead of re-reading the repo. Point
a coding agent at `http://localhost:7431/llms.txt` and it has the API, the MCP
tools, the bridge manual and the glossary — versioned together.

## API

Base `http://localhost:7431`, everything under `/v1`.

| Method | Endpoint | What it does | Redis |
| --- | --- | --- | --- |
| `POST` | `/v1/scrape` | Page → markdown. `smart` / `static` / `dynamic`, screenshots, `extract_schema`, `transport` | No |
| `POST` | `/v1/search` | Native 21-engine roster → SearXNG → stealth → **your browser** → Brave | No |
| `POST` | `/v1/research` | One query → search results **and** the pages behind them | No |
| `POST` | `/v1/map` | Sitemap → robots.txt → link fallback | No |
| `POST` | `/v1/crawl` · `/v1/batch/scrape` · `/v1/monitor` | Async BFS crawl, 20-URL batch, change-tracking webhooks | **Yes** |

Blocked pages answer `502 {"code":"blocked","vendor":"cloudflare"}` — never the challenge body as markdown. Params, error codes and the full reference: [`docs/guides/API_REFERENCE.md`](docs/guides/API_REFERENCE.md) and [`packages/api/README.md`](packages/api/README.md#endpoints).

## Borrow your own browser

Sites that block datacenter traffic can't be scraped honestly — so don't scrape them. Install the [bridge extension](./packages/bridge), pair it once, and ask for the browser:

```bash
curl -X POST http://localhost:7431/v1/scrape \
  -d '{"url":"https://example.com/dashboard","transport":"bridge"}'
```

**No recipe required.** A per-origin allowlist is the gate: the first touch answers `428 consent-required` naming the origin, and `tomoshibi bridge allow example.com` opens it. Results carry `provenance` and are marked `untrusted`, never cached, and every fetch lands in an access log.

Recipes are the specialization, not the price of admission — use one when a site needs its own actions, extraction schema, pace, or the `turbo` layer:

```bash
tomoshibi recipes add <git-url>#<ref> && tomoshibi recipes verify
```

## MCP

3 tools, not 8 — resource-oriented (`action` enum).

| Tool | `action` |
| --- | --- |
| `tomoshi_extract` | `scrape` · `scrape_multi` · `links` · `batch` · `batch_status` · `research` |
| `tomoshi_discover` | `search` · `map` · `crawl` · `crawl_status` |
| `tomoshi_monitor` | `create` · `status` · `delete` |

```jsonc
// .vscode/mcp.json or Claude config
{
  "mcpServers": {
    "tomoshi": {
      "command": "npx",
      "args": ["-y", "tomoshi"],
      "env": {
        "TOMOSHI_API_URL": "http://localhost:7431",
        // optional search keys — forwarded per request as X-Tomoshi-Env,
        // so configure them HERE, not in the backend's .env
        "BRAVE_SEARCH_API_KEY": "BSB-..." // or SERPER_API_KEY / TAVILY_API_KEY
      }
    }
  }
}
```

Full tool reference, `scrape` params and the shell-out story: [`packages/mcp/README.md`](./packages/mcp/README.md).

## Web

Svelte 5 playground for scrape/crawl/search — same API, browser UI.

```bash
bun --cwd packages/web dev   # http://localhost:5173
```

Details: [`packages/web/README.md`](./packages/web/README.md).

---

## Verified

Every number below was produced by running the thing, not by estimating it.

| Surface | How it was checked |
| --- | --- |
| API | 23 Go packages under `go test ./...`, `-race` in CI, `gofmt` + `go vet` + `staticcheck` clean |
| Extension | 27 unit tests (happy-dom), E2E-A protocol suite, E2E-B **19/19** in real Chrome |
| MCP | 63 tests, `tsc -p tsconfig.build.json` build |
| Live | `bun test/live-research.ts` — real DuckDuckGo, Google and Bing through a real browser, plus a research fan-out with `paid_api_used: false` |

Versions: repo **1.1.0** · api **1.1** · bridge **0.2.0** · mcp **1.2.0** · web **0.1.0**.

## Agent Skills

An installable skill for AI coding agents, indexed by [skills.sh](https://skills.sh) and the [`skills` CLI](https://github.com/vercel-labs/skills) (75+ agents).

```bash
npx skills add Michael-Obele/tomoshibi -g      # all projects
npx skills add Michael-Obele/tomoshibi         # this project only
```

Source: [`skills/tomoshibi`](skills/tomoshibi) — mirrored to `.agents/skills/` and `.agent/skills/` for OpenCode/Cursor.

---

## Name

**Tomoshibi** (灯火 — ともしび) — Japanese for _lamp light_, the glow of a paper lantern. Search as illumination. Short alias **tomoshi** (灯). Formerly **Cinder** (extinct fire — wrong metaphor, and the name collided with two unrelated projects).

## License

MIT — see [LICENSE](LICENSE).

**One exception, by directory.** [`packages/bridge/`](./packages/bridge) is PolyForm Noncommercial 1.0.0 — see [packages/bridge/LICENSE](./packages/bridge/LICENSE). Read it, run it, fork it for yourself; do not sell it. Everything else stays MIT.

> **Migrating from Cinder?** `github.com/Michael-Obele/cinder` redirects here. Images `ghcr.io/michael-obele/cinder` → `…/tomoshibi`. Only the import path changes (`github.com/Michael-Obele/tomoshibi`).
