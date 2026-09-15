# Tomoshibi vs Exa — Parity Analysis (2026-08-30)

Research target: make Tomoshibi's MCP surface (via `tomoshibi-mcp`) as good as or
better than the **Exa MCP server** (`exa-labs/exa-mcp-server`). This document
maps Exa's capabilities against Tomoshibi's, records where each wins, and lists
the concrete gaps to close. Research was done against Exa's public docs and
the live `web_search_exa` / `web_fetch_exa` tools.

## Exa MCP surface

| Tool                      | What it does                                                                                                                                                                                                     |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `web_search_exa`          | Semantic search (vector embeddings). Query → nearest neighbors in embedding space. Returns clean, ready-to-use content. Supports inline `category:` filters (company, publication, news, people, personal site). |
| `web_fetch_exa`           | Read one or more URLs as clean markdown.                                                                                                                                                                         |
| `web_search_advanced_exa` | Full Search API: category filters, domain restrictions, date ranges, geo-targeting, text constraints, query expansion, **summaries**, **highlights**, freshness control, **subpage crawling**.                   |
| `agent_run`               | Multi-step research agent: list-building, enrichment, structured output.                                                                                                                                         |

Exa's core differentiator: **semantic search over its own index**, plus
highlights and summaries that make results immediately usable by an LLM.

## Tomoshibi MCP surface (tomoshibi-mcp)

| Tool                                   | What it does                                                                                                                                                                                                                                         |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cinder_scrape`                        | Smart/static/dynamic scrape → clean LLM-ready markdown. Screenshots, image extraction (url/blob + resize/re-encode), extractive summary, deterministic CSS-selector schema extraction, PII redaction, page actions (click/wait/scroll), ad blocking. |
| `cinder_crawl` / `cinder_crawl_status` | Async BFS crawl: depth/limit, include/exclude globs, webhooks, per-domain politeness, retry w/ backoff.                                                                                                                                              |
| `cinder_search`                        | Self-hosted SearXNG (aggregates Google/Bing/DDG/Brave/Mojeek/Wikipedia) + optional Brave fallback. Pagination, domain include/exclude, requiredText, maxAge, fast mode.                                                                              |
| `cinder_monitor`                       | Change-tracking: hash page markdown, fire signed webhook on change, on an interval.                                                                                                                                                                  |

## Parity table

| Capability                             | Exa                               | Tomoshibi                                     | Winner     |
| -------------------------------------- | --------------------------------- | ------------------------------------------ | ---------- |
| Semantic (vector) search               | ✅                                | ❌ (keyword via SearXNG/Brave)             | **Exa**    |
| Search highlights / excerpts           | ✅                                | ❌                                         | **Exa**    |
| Search summaries                       | ✅ (advanced)                     | ✅ (extractive, LLM-free)                  | Tie        |
| Clean markdown fetch                   | ✅                                | ✅ (readability + html-to-markdown)        | Tie        |
| JS rendering (SPAs)                    | ❌ (static fetch)                 | ✅ (Chromedp)                              | **Tomoshibi** |
| Screenshots                            | ❌                                | ✅                                         | **Tomoshibi** |
| Image extraction                       | ❌                                | ✅ (srcset/picture/lazy, resize/re-encode) | **Tomoshibi** |
| Structured extraction (CSS selectors)  | ❌                                | ✅                                         | **Tomoshibi** |
| PII redaction                          | ❌                                | ✅                                         | **Tomoshibi** |
| Page actions (click/wait/scroll)       | ❌                                | ✅                                         | **Tomoshibi** |
| Multi-page crawling                    | Partial (subpage crawl in search) | ✅ (BFS, globs, webhooks)                  | **Tomoshibi** |
| Change-tracking monitors               | ❌                                | ✅                                         | **Tomoshibi** |
| Batch URL fetch                        | ✅ (multi-URL fetch)              | ✅ (async batch)                           | Tie        |
| Category filters (company/news/people) | ✅                                | ❌                                         | **Exa**    |
| Date-range / freshness filters         | ✅                                | Partial (maxAge)                           | **Exa**    |
| Domain filters                         | ✅                                | ✅ (include/exclude)                       | Tie        |
| Self-hosted / no per-request cost      | ❌ (paid API)                     | ✅                                         | **Tomoshibi** |
| Multi-step research agent              | ✅ (`agent_run`)                  | ❌ (client-side)                           | **Exa**    |

## Verdict

Tomoshibi already **beats Exa on content fidelity**: it renders JS, captures
screenshots, extracts images, does structured extraction, redacts PII, and
crawls — none of which Exa's fetch does. For "turn a URL into clean,
LLM-ready content", Tomoshibi is strictly more capable.

The gap is on the **search side**: Exa's semantic search, highlights, and
category/date filters are its moat. Tomoshibi's search is keyword-based
(SearXNG/Brave) — SearXNG aggregates many engines so it is stable and free,
but it is still keyword matching, not vector similarity.

## What this sprint shipped to close the gap

1. **Self-hosted SearXNG backend** (`internal/search/searxng.go`) — replaces
   the fragile direct DuckDuckGo scraper. SearXNG aggregates Google, Bing,
   DDG, Brave, Mojeek and Wikipedia, so search no longer depends on scraping
   a single rate-limited engine. The old DuckDuckGo scraper was removed.
2. **Search result caching** (`internal/search/cache.go`) — repeat queries
   served from Redis in ~2ms instead of re-hitting the upstream engine. Under
   a 45s/20-worker load test, `/v1/search` went from **18% → 100% success**
   and stays at 100% with SearXNG.
3. **Load harness** (`scripts/load-harness.go`) — per-endpoint status
   distributions, latency percentiles (p50/p95/p99), error classification
   (dns/timeout/conn-refused/conn-reset), a false-404 detector that hammers
   valid IDs under load, and a crawl lifecycle test that flags any 404 on a
   valid task ID.

## Roadmap to full parity (search side)

- **Highlights**: return query-term-highlighted snippets per result (SearXNG
  already returns snippets; extract the matching span around query terms).
- **Category filters**: map Exa's `category:` (company/publication/news/
  people) onto SearXNG's `categories` param (`general`, `news`, `it`, ...)
  and Brave's `search_type` / domain heuristics.
- **Semantic search**: the only true gap. Options, in increasing effort:
  1. Client-side re-ranking: embed the query + result snippets with a local
     model (e.g. `bge-small` via ONNX) and re-rank — no index needed.
  2. Self-hosted vector index (SQLite-vec / Qdrant) fed by the crawl/map
     pipeline, queried by embedding similarity.
  3. Optional plug-in to a hosted embeddings API for those who want it.
- **Multi-URL fetch**: add a synchronous `POST /v1/scrape/batch` (or accept
  `urls: []` on `/v1/scrape`) so one call mirrors `web_fetch_exa`'s
  multi-URL behavior without the async round-trip.

## Notes

- The MCP layer itself lives in `tomoshibi-mcp` (TMCP + Bun) and was **not**
  changed in this sprint; backend changes here flow through automatically.
- Exa's `agent_run` is an LLM-orchestration feature, not a scraping feature;
  parity there is a client concern (the AI assistant already provides the
  orchestration), not a backend one.
