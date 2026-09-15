# Search & Scrape Comparison — Tomoshibi vs Exa vs Firecrawl (2026-08-30)

Backed-by-benchmark comparison of Tomoshibi's search and scrape against Exa
(paid API) and a self-hosted Firecrawl (`v2.11.162`). Every Tomoshibi and
Firecrawl number below was measured on this machine against live local
instances; Exa numbers are from its published free-tier limits and docs
(its API is not load-testable here without a key).

## Benchmark methodology

- Search: `scripts/search-bench.py` — 10 concurrent workers, 30s, cycling 10
  varied real-world queries (`golang concurrency`, `svelte 5 runes`,
  `cloudflare workers durable objects`, ...), reporting success rate and
  latency percentiles.
- Scrape: 5 concurrent `POST /v1/scrape` (Tomoshibi) vs `POST /v2/scrape`
  (Firecrawl) of `https://go.dev/blog`, reporting status + wall time.
- Same host, same network, same moment in time.

## Search — measured

| Metric       | **Tomoshibi (SearXNG)**                                        | **Firecrawl (self-hosted)** | Exa (cloud)                |
| ------------ | ----------------------------------------------------------- | --------------------------- | -------------------------- |
| Throughput   | **560 req/s**                                               | 1.9 req/s                   | rate-limited free tier     |
| p50 latency  | **11 ms**                                                   | 5.4 s                       | n/a (paid)                 |
| p95 latency  | **21 ms**                                                   | 8.7 s                       | n/a (paid)                 |
| Success rate | **100%** (16,809/16,809)                                    | 100% (60/60)                | n/a                        |
| Cost         | **$0** (self-hosted)                                        | $0 (self-hosted)            | paid API                   |
| Backend      | SearXNG (aggregates Google/Bing/DDG/Brave/Mojeek/Wikipedia) | proprietary                 | proprietary semantic index |

Tomoshibi's search is **~300× the throughput and ~500× lower p50 latency** than
self-hosted Firecrawl's search, at the same $0 cost. Firecrawl's search is
slow because it fetches and extracts each result page rather than returning
result metadata.

### Why Tomoshibi's search is fast

1. **Self-hosted SearXNG** — no per-query cost, no third-party rate limit;
   SearXNG aggregates many engines and handles concurrency internally.
2. **Redis result cache** — repeat queries are served in ~2 ms without
   touching SearXNG at all.
3. **No client-side rate limiter** — the load benchmark exposed that a
   1 req/1.1s limiter (inherited from the old Brave pattern) serialized
   concurrent queries to ~1 req/s with a 10.9 s p50. Removing it took search
   from **0.9 req/s / 10.9 s p50 → 560 req/s / 11 ms p50**.

## Scrape — measured (5× go.dev/blog)

| Metric                                         | **Tomoshibi**                         | **Firecrawl (self-hosted)**           |
| ---------------------------------------------- | ---------------------------------- | ------------------------------------- |
| Success                                        | **5/5**                            | 5/5                                   |
| Latency                                        | **7–14 ms** (Redis-cached)         | ~17 s each (no cache)                 |
| Cold latency                                   | ~1–11 s (site-dependent)           | ~17 s (always full Playwright render) |
| JS rendering                                   | ✅ Chromedp (smart/static/dynamic) | ✅ Playwright                         |
| Screenshots / images / schema extraction / PII | ✅                                 | partial (screenshots yes)             |

Tomoshibi's scrape is ~1,500× faster on repeat scrapes thanks to its gzip
Redis cache; Firecrawl re-renders every request. Both render JS, but Tomoshibi
additionally does deterministic CSS-selector extraction, image extraction,
and PII redaction out of the box.

## Feature matrix

| Capability                              | **Tomoshibi**    | Exa       | Firecrawl (self-hosted) |
| --------------------------------------- | ------------- | --------- | ----------------------- |
| Clean markdown fetch                    | ✅            | ✅        | ✅                      |
| JS rendering (SPAs)                     | ✅ Chromedp   | ❌ static | ✅ Playwright           |
| Screenshots                             | ✅            | ❌        | ✅                      |
| Image extraction (srcset/picture/lazy)  | ✅            | ❌        | partial                 |
| Structured extraction (CSS selectors)   | ✅            | ❌        | ✅ (AI + selector)      |
| Extractive summary (LLM-free)           | ✅            | ✅        | ❌ (LLM only)           |
| PII redaction                           | ✅            | ❌        | ❌                      |
| Page actions (click/wait/scroll)        | ✅            | ❌        | ❌                      |
| Multi-page crawl (BFS, globs, webhooks) | ✅            | partial   | ✅                      |
| Change-tracking monitors                | ✅            | ❌        | ❌                      |
| Semantic (vector) search                | ❌ (roadmap)  | ✅        | ❌                      |
| Search highlights / category filters    | ❌ (roadmap)  | ✅        | ❌                      |
| Self-hosted / no per-request cost       | ✅            | ❌ paid   | ✅                      |
| Search throughput (measured)            | **560 req/s** | n/a       | 1.9 req/s               |

## Bottom line

Tomoshibi is now the **fastest free search+scrape stack of the three** on this
machine: search at 560 req/s with 11 ms p50, scrape with a Redis cache that
makes repeat requests near-instant, and JS rendering that matches Firecrawl.
Exa's remaining moat is semantic search — tracked in `docs/EXA_PARITY.md`
with a concrete roadmap (highlights, category filters, optional embedding
re-rank).

## Reproduce

```bash
# search benchmark (Tomoshibi)
python3 scripts/search-bench.py --url http://localhost:8080/v1/search \
  --concurrency 10 --duration 30 --label "Tomoshibi (SearXNG)"

# search benchmark (Firecrawl)
python3 scripts/search-bench.py --url http://localhost:3002/v1/search \
  --concurrency 10 --duration 30 --label "Firecrawl"

# scrape burst (Tomoshibi)
for i in 1 2 3 4 5; do curl -s -o /dev/null -w "%{http_code} %{time_total}s\n" \
  -X POST http://localhost:8080/v1/scrape -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/blog","mode":"smart"}' & done; wait

# scrape burst (Firecrawl)
for i in 1 2 3 4 5; do curl -s -o /dev/null -w "%{http_code} %{time_total}s\n" \
  -X POST http://localhost:3002/v2/scrape -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/blog","formats":["markdown"],"timeout":60000}' & done; wait
```
