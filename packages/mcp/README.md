# tomoshi (灯 — lamp light)

Stop paying for Firecrawl. Self-hosted web search & scraping for AI agents — one binary, $5/mo on Fly.io.

`tomoshi` turns any website into LLM-ready markdown, with search (SearXNG → Brave fallback), crawling, and change monitoring — all behind a 3-tool MCP.

## Why tomoshi?

- **Drop-in Firecrawl alternative** — same `scrape`/`crawl`/`search`/`map` semantics, no vendor lock-in
- **Own your data** — runs on your Fly.io / Docker host, not someone else's cloud
- **3 tools, not 17** — `cinder_extract` (scrape/multi/links/batch), `cinder_discover` (search/map/crawl), `cinder_monitor` (change tracking)
- **Provenance + OIDC** — every npm release is Sigstore-signed via GitHub Actions

## Install (30 seconds)

**Prerequisites:** A running Tomoshibi API (`docker compose up -d` or `fly deploy`). See [tomoshibi](https://github.com/Michael-Obele/tomoshibi) for hosting.

```json
{
  "mcpServers": {
    "tomoshi": {
      "command": "npx",
      "args": ["-y", "tomoshi"],
      "env": {
        "TOMOSHIBI_API_URL": "http://localhost:7431"
      }
    }
  }
}
```

Legacy `CINDER_API_URL` still works. `tomoshibi` and `tomoshibi-mcp` are alias bins.

## Tools

| Tool | Actions |
|------|---------|
| `cinder_extract` | `scrape` · `scrape_multi` (≤10 URLs) · `links` · `batch` · `batch_status` |
| `cinder_discover` | `search` · `map` (sitemap) · `crawl` · `crawl_status` |
| `cinder_monitor` | `create` · `status` · `delete` |

All async actions (`crawl`, `batch`, `monitor`) are Redis-backed — poll `*_status` until done.

## Links

- **GitHub:** https://github.com/Michael-Obele/tomoshibi
- **npm:** https://www.npmjs.com/package/tomoshi
- **API docs:** `https://your-api.fly.dev/docs` (Swagger, auto-generated)
