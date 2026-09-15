# Crawl v2 — Parallel, Polite, Retryable

> Feature spec — implemented 2026-08-01 (`internal/worker/crawl_handler.go`)

## What changed

### 1. Parallel worker pool

The BFS crawl now runs on a shared worker pool (`CRAWL_CONCURRENCY`, default 4,
max 10) instead of one page at a time. A page at depth N that links to 10
pages at depth N+1 has all 10 scraped concurrently.

### 2. Per-domain politeness

A minimum interval between requests to the **same host** is enforced across
all workers (`CRAWL_DOMAIN_DELAY`, default 1s). Different domains are not
throttled against each other.

### 3. Retry policy

- Transient failures (network, 5xx) are retried (`CRAWL_MAX_RETRIES`, default 2) with backoff (1s, 2s, …).
- **4xx errors are never retried** — recorded as failed immediately
  (`scraper.StatusError` carries the HTTP status through the stack). 429s are
  included: retrying without honoring `Retry-After` would just pile on more
  throttled requests.

### 4. Include/exclude patterns

`include_paths` / `exclude_paths` are glob patterns matched against the URL
path (gobwas/glob semantics: `*` stays within a segment, `**` crosses
segments, e.g. `/blog/**`). Exclusion wins; empty include = allow all.
**The seed URL always bypasses the filters** — patterns only apply to links
discovered while following pages.

### 5. Signed completion webhooks

`webhook_url` + `webhook_secret` POST the full `CrawlResult` on completion
with header `X-Tomoshibi-Signature: sha256=<hmac-hex>`. Three delivery attempts
with backoff; 4xx responses are not retried. Delivery failure never fails the
crawl — it's logged and visible via the task result.

### 6. Deadlock-free bounded queue

The work queue is **bounded and non-blocking**: when its buffer is full,
excess links are dropped (the `limit` check caps results anyway), so no
worker can ever block on enqueue and deadlock the pool. A global queue-depth
counter keeps the queue from carrying more than `limit` unprocessed entries.

### 7. Watchdog timeouts

- Internal deadline `CRAWL_TIMEOUT` (default 30m) — the crawl returns a
  `"timeout"` result instead of hanging an Asynq worker forever.
- Per-attempt scrape deadline `CRAWL_SCRAPE_TIMEOUT` (default 30s) so a slow
  site can't pin a worker between retries.
- The Asynq task itself is killed after `CRAWL_TIMEOUT + 5m` (retries then
  take over per `MaxRetry`) — a safety net for handler bugs.

## API

```json
{
  "url": "https://docs.example.com",
  "maxDepth": 3,
  "limit": 50,
  "include_paths": ["/docs/*"],
  "exclude_paths": ["/docs/internal/*", "/login"],
  "webhook_url": "https://myapp.example.com/hooks/tomoshibi",
  "webhook_secret": "s3cret"
}
```

## Env vars

| Var                    | Default | Purpose                                  |
| ---------------------- | ------- | ---------------------------------------- |
| `CRAWL_CONCURRENCY`    | 4       | Parallel workers (1-10)                  |
| `CRAWL_DOMAIN_DELAY`   | 1       | Seconds between same-host requests       |
| `CRAWL_TIMEOUT`        | 30      | Overall crawl deadline (minutes, 5-720)  |
| `CRAWL_SCRAPE_TIMEOUT` | 30      | Per-attempt scrape deadline (s, 5-300)   |
| `CRAWL_MAX_RETRIES`    | 2       | Retries per URL (0-5; 4xx never retried) |
| `WEBHOOK_TIMEOUT`      | 10      | Webhook HTTP timeout (s)                 |

## Concurrency correctness

- `visited` dedup, result/failure collection, queue-depth accounting, and
  politeness bookkeeping are mutex-guarded; queue closure is `sync.Once`-
  guarded. The queue is closed **only when the pending counter reaches 0** —
  at that point every worker has finished sending, so close can never race a
  send. Cancellation and limit-reached use a separate `stop` channel instead
  of closing the queue, so sends never panic on a closed channel.
- Result order is completion order (documented non-determinism).
