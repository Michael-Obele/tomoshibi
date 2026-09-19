---
name: tomoshibi
description: Use Tomoshibi (tomoshi) MCP to turn any site into LLM-ready markdown — scrape, search, map, crawl, batch and monitor. Use when fetching web content, searching the web, extracting links, crawling a site, or watching a page for changes via tomoshi_extract, tomoshi_discover, tomoshi_monitor.
license: MIT
metadata:
  author: Michael-Obele/tomoshibi
  version: "1.0.0"
  domain: web-scraping
  triggers: scrape, scraping, crawl, crawling, search, map, sitemap, monitor, markdown, tomoshi, tomoshibi, firecrawl, exa, web fetch
  role: specialist
  scope: implementation
  output-format: code
---

# Tomoshibi — lamp light for AI agents

Self-hosted Firecrawl/Exa alternative. **3 tools, not 17** — resource-oriented with `action` enum. One binary, $5/mo hobby-tier.

## When to Use This Skill

- User wants to **scrape** a URL to markdown, screenshot, images, or structured data
- User wants to **search** the web (SearXNG + Brave fallback)
- User wants to **map** a site (sitemap → robots.txt → link fallback)
- User wants to **crawl** a site async (BFS, Redis-backed)
- User wants to **batch** scrape 2–20 URLs async
- User wants to **monitor** a page for changes (hash + webhook)
- User mentions `tomoshi`, `tomoshibi`, `cinder`, `firecrawl`, `exa`, or "turn site into markdown"

## Prerequisites

A running Tomoshibi API is required. The MCP is a thin client over HTTP.

```bash
# fastest — full stack (api 7431 + redis 7434 + searxng 7435 + mcp 7433 + web 7432)
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
docker compose up -d
curl http://localhost:7431/health  # → {"status":"ok"}

# or single binary
go run ./packages/api/cmd/api
REDIS_URL=redis://localhost:7434 go run ./packages/api/cmd/api  # with async
```

MCP config (`.vscode/mcp.json` or Claude config):

```jsonc
{
  "mcpServers": {
    "tomoshi": {
      "command": "npx",
      "args": ["-y", "tomoshi"],
      "env": { "TOMOSHIBI_API_URL": "http://localhost:7431" },
      // legacy CINDER_API_URL also works
    },
  },
}
```

Aliases: `npx tomoshi`, `npx tomoshibi`, `npx tomoshibi-mcp` — same package.

## Tools (3)

| Tool               | `action`                                                       | Endpoint                             | Needs Redis   |
| ------------------ | -------------------------------------------------------------- | ------------------------------------ | ------------- |
| `tomoshi_extract`  | `scrape` · `scrape_multi` · `links` · `batch` · `batch_status` | `/v1/scrape`, `/v1/batch`            | `batch*` only |
| `tomoshi_discover` | `search` · `map` · `crawl` · `crawl_status`                    | `/v1/search`, `/v1/map`, `/v1/crawl` | `crawl*` only |
| `tomoshi_monitor`  | `create` · `status` · `delete`                                 | `/v1/monitor`                        | Yes           |

> Async actions (`batch`, `crawl`, `monitor`) are Redis-backed — poll `*_status` until done.

### 1. tomoshi_extract — extraction resource

```ts
// single page → markdown (smart is default: static Colly first, fallback to Chromedp)
{ action: "scrape", url: "https://example.com", mode: "smart" }

// sync multi (≤10, no Redis, errgroup 5)
{ action: "scrape_multi", urls: ["https://a.com", "https://b.com"], mode: "smart" }

// links only
{ action: "links", url: "https://example.com" }

// async batch (≤20, Redis) → poll
{ action: "batch", urls: ["https://a.com", "..."] }  // → { id }
{ action: "batch_status", id: "<batch-id>" }

// common scrape options (all actions that scrape)
{
  mode: "smart" | "static" | "dynamic", // static=Colly, dynamic=Chromedp, smart=auto
  screenshot: true, screenshot_opts: { full_page: true, format: "jpeg", quality: 80 },
  images: true, image_format: "url" | "blob", max_images: 10,
  extract_schema: { title: { selector: "h1" }, price: { selector: ".price" } },
  summary: true, summary_sentences: 5,
  redact_pii: true, block_ads: true,
  actions: [{ type: "wait_selector", selector: "#app" }, { type: "scroll_to_bottom" }]
}
```

### 2. tomoshi_discover — discovery resource

```ts
// search (SearXNG, Brave fallback)
{ action: "search", query: "svelte 5 runes", limit: 10, offset: 0, category: "general" | "news" | "code", maxAge: 1|7|30, includeDomains: ["svelte.dev"], excludeDomains: ["spam.com"] }

// map (sitemap discovery)
{ action: "map", url: "https://example.com", search: "/docs", limit: 100 }

// async crawl (BFS, Redis) → poll
{ action: "crawl", url: "https://example.com", mode: "smart", maxDepth: 2, limit: 10, include_paths: ["/blog/*"], exclude_paths: ["/admin/*"] } // → { id }
{ action: "crawl_status", id: "<crawl-id>" }
```

### 3. tomoshi_monitor — change tracking (Redis)

```ts
{ action: "create", url: "https://example.com/pricing", interval_seconds: 3600, webhook_url: "https://my.app/hook", webhook_secret: "hmac-key" } // → { id }
{ action: "status", id: "<monitor-id>" }
{ action: "delete", id: "<monitor-id>" }
```

## Workflows

### Research → scrape (most common)

```
tomoshi_discover { action: "search", query: "svelte 5 runes", limit: 5 }
→ tomoshi_extract { action: "scrape_multi", urls: [top 3 urls] }
→ markdown ready for LLM
```

### Crawl a docs site

```
tomoshi_discover { action: "map", url: "https://example.com", search: "/docs" }
→ tomoshi_discover { action: "crawl", url: "https://example.com", include_paths: ["/docs/*"], limit: 20 }
→ poll tomoshi_discover { action: "crawl_status", id }
```

### Watch for changes

```
tomoshi_monitor { action: "create", url: "https://example.com/changelog", interval_seconds: 3600 }
→ tomoshi_monitor { action: "status", id }
```

## Constraints

- **MUST** use `action` enum — there are only 3 tools. Do not invent `cinder_*` or `firecrawl_*` names.
- **MUST** poll `*_status` for async jobs; they return `{ id }` immediately.
- **MUST** respect limits: `scrape_multi` ≤10, `batch` ≤20, `map` ≤5000, `crawl` limit ≤100, depth ≤10.
- **MUST** handle Redis-optional degradation: without Redis, `crawl`/`batch`/`monitor` return 503 — fall back to `scrape`/`scrape_multi`/`map`/`search`.
- **MUST NOT** hardcode `TOMOSHIBI_API_URL` — read from env/mcp config.

## References

- API: `http://localhost:7431` — all endpoints under `/v1` (`/scrape`, `/search`, `/map`, `/crawl`, `/batch`, `/monitor`)
- Docs: `packages/mcp/README.md`, `docs/guides/API_REFERENCE.md`
- Swagger (debug mode): `http://localhost:7431/docs`
