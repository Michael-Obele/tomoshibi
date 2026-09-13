package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gobwas/glob"
	"github.com/hibiken/asynq"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

// CrawlResult is the aggregated output of a multi-page crawl.
type CrawlResult struct {
	Status     string                `json:"status"` // "completed", "partial"
	TotalPages int                   `json:"total"`
	MaxDepth   int                   `json:"maxDepth"`
	Limit      int                   `json:"limit"`
	Pages      []domain.ScrapeResult `json:"data"`
	FailedURLs []FailedURL           `json:"failedUrls,omitempty"`
}

// FailedURL records a URL that could not be scraped during the crawl.
type FailedURL struct {
	URL   string `json:"url"`
	Error string `json:"error"`
}

// CrawlTaskHandler processes multi-page crawl tasks using BFS.
type CrawlTaskHandler struct {
	scraper *scraper.Service
	logger  *slog.Logger
}

func NewCrawlTaskHandler(scraper *scraper.Service, logger *slog.Logger) *CrawlTaskHandler {
	return &CrawlTaskHandler{
		scraper: scraper,
		logger:  logger,
	}
}

// ProcessTask is the Asynq entry point — it deserializes the payload,
// delegates to ExecuteCrawl, and writes the result back to Asynq.
func (h *CrawlTaskHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload CrawlPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal crawl payload: %w, task_id=%s", err, t.ResultWriter().TaskID())
	}

	taskID := t.ResultWriter().TaskID()

	result, err := h.ExecuteCrawl(ctx, payload, taskID)
	if err != nil {
		return err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal crawl result: %w", err)
	}

	if _, err := t.ResultWriter().Write(resultJSON); err != nil {
		h.logger.Error("Failed to write crawl result", "error", err, "task_id", taskID)
	}

	// Fire the completion webhook (best-effort; failures are logged only).
	if payload.WebhookURL != "" {
		if err := Deliver(ctx, payload.WebhookURL, payload.WebhookSecret, resultJSON); err != nil {
			h.logger.Warn("Webhook delivery failed", "url", payload.WebhookURL, "error", err, "task_id", taskID)
		} else {
			h.logger.Info("Webhook delivered", "url", payload.WebhookURL, "task_id", taskID)
		}
	}

	return nil
}

// ExecuteCrawl performs a parallel BFS crawl. Pages at the same depth are
// scraped concurrently (up to crawlConcurrency workers) while respecting a
// per-domain politeness delay, retrying transient failures with backoff,
// and never retrying 4xx errors.
//
// Semantics worth noting:
//   - The seed URL is ALWAYS scraped, even when include/exclude patterns are
//     set — patterns apply only to links discovered while following pages.
//   - The crawl is bounded by an internal deadline (CRAWL_TIMEOUT); on
//     expiry the result status is "timeout" so a misbehaving site can never
//     hang the worker forever.
//   - Enqueuing is non-blocking: when the work queue is full, excess links
//     are dropped. The queue therefore never carries more than `limit`
//     unprocessed entries, and no producer can block on a full buffer.
func (h *CrawlTaskHandler) ExecuteCrawl(ctx context.Context, payload CrawlPayload, taskID string) (*CrawlResult, error) {
	// Apply sensible defaults and caps
	maxDepth := payload.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 2
	}
	if maxDepth > 10 {
		maxDepth = 10
	}

	limit := payload.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Resolve scraping mode
	mode := payload.Mode
	if payload.Render {
		mode = "dynamic"
	}
	if mode == "" {
		mode = "smart"
	}

	// Internal watchdog: a misbehaving site must never occupy an Asynq
	// worker indefinitely. The task-level timeout (asynq.Timeout) is a
	// larger safety net for handler bugs; this deadline guarantees the
	// handler itself always returns.
	crawlCtx, cancel := context.WithTimeout(ctx, crawlTimeout())
	defer cancel()

	seedURL, err := url.Parse(payload.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid seed URL %q: %w", payload.URL, err)
	}
	allowedHost := seedURL.Hostname()

	h.logger.Info("Starting crawl",
		"timeout", crawlTimeout().String(),
		"url", payload.URL,
		"maxDepth", maxDepth,
		"limit", limit,
		"mode", mode,
		"concurrency", crawlConcurrency(),
		"task_id", taskID,
	)

	scrapeOpts := domain.ScrapeOptions{
		Screenshot: payload.Screenshot,
		Images:     payload.Images,
	}
	switch payload.ImageFormat {
	case "blob":
		scrapeOpts.ImageFormat = domain.ImageFormatBlob
	case "url":
		scrapeOpts.ImageFormat = domain.ImageFormatURL
	}

	type queueEntry struct {
		url   string
		depth int
	}

	// Buffered work queue. Enqueue is NON-BLOCKING: when the buffer is full
	// the entry is simply dropped (the limit check caps results anyway), so
	// no producer can ever block and deadlock the pool. Dropped entries just
	// mean BFS visits fewer pages than the frontier offered.
	queue := make(chan queueEntry, crawlConcurrency()*2)

	// stop signals workers to exit early (cancellation or limit reached).
	// queue is closed ONLY when pending hits 0 — at that point every worker
	// has finished sending, so close can never race a send.
	var stopOnce sync.Once
	stop := make(chan struct{})
	closeStop := func() { stopOnce.Do(func() { close(stop) }) }

	var mu sync.Mutex
	visited := map[string]bool{normalizeURL(payload.URL): true}
	results := []domain.ScrapeResult{}
	failed := []FailedURL{}
	lastRequest := make(map[string]time.Time) // per-host politeness
	pending := 1                              // the seed entry, not yet picked
	queued := 1                               // the seed entry, still in the queue

	var closeOnce sync.Once
	closeQueue := func() { closeOnce.Do(func() { close(queue) }) }

	// Workers exit early when the crawl is cancelled. The watchdog goroutine
	// is joined (watchDone) before ExecuteCrawl returns, so it can never
	// outlive the call and trip the race detector.
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		<-crawlCtx.Done()
		closeStop()
	}()

	queue <- queueEntry{url: normalizeURL(payload.URL), depth: 0}

	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for {
			var entry queueEntry
			var ok bool
			select {
			case entry, ok = <-queue:
				if !ok {
					// queue closed: every entry has been picked (close only
					// happens when pending reaches 0) — crawl is done.
					return
				}
			case <-stop:
				return
			}
			if crawlCtx.Err() != nil {
				return
			}

			mu.Lock()
			queued--
			overLimit := len(results) >= limit
			mu.Unlock()
			if overLimit {
				closeStop()
				return
			}

			// Per-domain politeness: enforce a minimum interval between
			// requests to the same host across all workers.
			host := hostOf(entry.url)
			delay := time.Duration(0)
			mu.Lock()
			if last, ok := lastRequest[host]; ok {
				delay = domainDelay() - time.Since(last)
				if delay < 0 {
					delay = 0
				}
			}
			lastRequest[host] = time.Now().Add(delay)
			mu.Unlock()
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-crawlCtx.Done():
					return
				}
			}

			// Snapshot under the lock: len(results) is written by other
			// workers and must not be read unlocked.
			mu.Lock()
			scraped := len(results)
			mu.Unlock()

			h.logger.Info("Crawling page",
				"url", entry.url,
				"depth", entry.depth,
				"scraped", scraped,
				"queued", len(queue),
				"task_id", taskID,
			)

			result, scrapeErr := scrapeWithRetry(crawlCtx, h.scraper, entry.url, mode, scrapeOpts)
			if scrapeErr != nil {
				mu.Lock()
				failed = append(failed, FailedURL{URL: entry.url, Error: scrapeErr.Error()})
				pending--
				empty := pending == 0
				mu.Unlock()
				if empty {
					closeQueue()
				}
				continue
			}

			mu.Lock()
			if len(results) >= limit {
				mu.Unlock()
				closeStop()
				return
			}
			results = append(results, *result)
			mu.Unlock()

			// If we haven't reached maxDepth, extract links and enqueue.
			if entry.depth < maxDepth {
				links := filterByPatterns(extractLinks(result.HTML, entry.url, allowedHost),
					payload.IncludePaths, payload.ExcludePaths)

				newEntries := make([]queueEntry, 0, len(links))
				mu.Lock()
				for _, link := range links {
					normalized := normalizeURL(link)
					if visited[normalized] {
						continue
					}
					// Global queue-depth cap: stop feeding once the crawl is
					// already at the limit, so the queue can never carry a
					// large multiple of `limit` worth of wasted scrapes.
					if len(results)+queued >= limit {
						break
					}
					visited[normalized] = true
					pending++
					queued++
					newEntries = append(newEntries, queueEntry{url: normalized, depth: entry.depth + 1})
				}
				mu.Unlock()

				for _, e := range newEntries {
					select {
					case queue <- e:
					case <-stop:
						return
					default:
						// Buffer full — drop the entry and undo its accounting.
						mu.Lock()
						queued--
						pending--
						mu.Unlock()
					}
				}
			}

			mu.Lock()
			pending--
			empty := pending == 0
			mu.Unlock()
			if empty {
				closeQueue()
			}
		}
	}

	for range crawlConcurrency() {
		wg.Add(1)
		go worker()
	}
	wg.Wait()

	// Capture the termination reason BEFORE waking the watchdog: cancel()
	// below makes crawlCtx.Err() report cancellation unconditionally.
	ctxErr := crawlCtx.Err()

	// Wake the watchdog (it blocks on crawlCtx.Done) and join it so no
	// goroutine outlives this call. cancel() is deferred as a safety net;
	// it is idempotent.
	cancel()
	<-watchDone

	status := "completed"
	if ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			status = "timeout"
		} else {
			status = "cancelled"
		}
	} else if len(failed) > 0 && len(results) == 0 {
		status = "failed"
	} else if len(failed) > 0 {
		status = "partial"
	}

	crawlResult := &CrawlResult{
		Status:     status,
		TotalPages: len(results),
		MaxDepth:   maxDepth,
		Limit:      limit,
		Pages:      results,
		FailedURLs: failed,
	}

	h.logger.Info("Crawl completed",
		"url", payload.URL,
		"totalPages", len(results),
		"failedPages", len(failed),
		"status", status,
		"task_id", taskID,
	)

	return crawlResult, nil
}

// scrapeWithRetry scrapes a URL with up to crawlMaxRetries retries and
// exponential backoff (1s, 2s, …) for transient failures. Each attempt is
// bounded by scrapeTimeout (CRAWL_SCRAPE_TIMEOUT) so a slow site can't pin
// a worker. 4xx errors are never retried (429s included — retrying without
// honoring Retry-After would just pile on more throttled requests).
func scrapeWithRetry(
	ctx context.Context,
	svc *scraper.Service,
	url, mode string,
	opts domain.ScrapeOptions,
) (*domain.ScrapeResult, error) {
	maxRetries := crawlMaxRetries()
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * retryBackoffUnit):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, scrapeTimeout())
		result, err := svc.Scrape(attemptCtx, url, mode, opts)
		cancel()
		if err == nil {
			return result, nil
		}
		lastErr = err
		var statusErr *scraper.StatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode >= 400 && statusErr.StatusCode < 500 {
			return nil, err
		}
	}
	return nil, lastErr
}

// crawlConcurrency returns the worker pool size (env CRAWL_CONCURRENCY,
// default 4, capped at 10).
func crawlConcurrency() int {
	return clampEnvInt("CRAWL_CONCURRENCY", 4, 1, 10)
}

// domainDelay returns the minimum interval between requests to the same
// host (env CRAWL_DOMAIN_DELAY seconds, default 1s).
func domainDelay() time.Duration {
	return time.Duration(clampEnvInt("CRAWL_DOMAIN_DELAY", 1, 0, 60)) * time.Second
}

// crawlTimeout returns the internal crawl deadline (env CRAWL_TIMEOUT
// minutes, default 30, clamped to 5–720).
func crawlTimeout() time.Duration {
	return time.Duration(clampEnvInt("CRAWL_TIMEOUT", 30, 5, 720)) * time.Minute
}

// scrapeTimeout returns the per-attempt scrape deadline inside a crawl
// (env CRAWL_SCRAPE_TIMEOUT seconds, default 30, clamped to 5–300).
func scrapeTimeout() time.Duration {
	return time.Duration(clampEnvInt("CRAWL_SCRAPE_TIMEOUT", 30, 5, 300)) * time.Second
}

// crawlMaxRetries returns the number of retries per URL (env
// CRAWL_MAX_RETRIES, default 2, clamped to 0–5).
func crawlMaxRetries() int {
	return clampEnvInt("CRAWL_MAX_RETRIES", 2, 0, 5)
}

// clampEnvInt reads an integer env var, returning fallback when unset or
// invalid, clamped to [min, max].
func clampEnvInt(key string, fallback, min, max int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}

// hostOf returns the hostname of a URL (empty on parse failure).
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// filterByPatterns applies include/exclude glob patterns (matched against
// the URL path). Exclusions win; an empty include list allows everything.
//
// Patterns support the full glob syntax of gobwas/glob: `*` matches within
// a path segment, `**` crosses segments (e.g. `/blog/**`), and `?`, `[abc]`,
// `{a,b}` work as usual. Note that the seed URL bypasses these filters — it
// is always scraped.
func filterByPatterns(links []string, include, exclude []string) []string {
	if len(include) == 0 && len(exclude) == 0 {
		return links
	}
	out := make([]string, 0, len(links))
	for _, link := range links {
		u, err := url.Parse(link)
		if err != nil {
			continue
		}
		p := u.Path
		if matchesAny(exclude, p) {
			continue
		}
		if len(include) > 0 && !matchesAny(include, p) {
			continue
		}
		out = append(out, link)
	}
	return out
}

// matchesAny reports whether p matches any glob pattern. Patterns are
// compiled with '/' as the path separator (gobwas/glob): `*` stays within a
// segment, `**` crosses segments — so `/blog/*` matches `/blog/post` but not
// `/blog/a/b/post`, while `/blog/**` matches both (and `/blog/`). Note that
// `/blog/**` does not match the bare `/blog` path (the pattern carries a
// trailing slash); use `/blog` or `/blog{,/**}` for that.
func matchesAny(patterns []string, p string) bool {
	for _, pat := range patterns {
		g, err := glob.Compile(pat, '/')
		if err != nil {
			continue
		}
		if g.Match(p) {
			return true
		}
	}
	return false
}

// extractLinks parses HTML and returns same-domain, non-resource links.
func extractLinks(htmlBody string, pageURL string, allowedHost string) []string {
	if htmlBody == "" {
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return nil
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}

	links := []string{}
	seen := make(map[string]bool)

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		// Skip non-HTTP schemes (mailto:, tel:, javascript:, #anchors)
		if strings.HasPrefix(href, "mailto:") ||
			strings.HasPrefix(href, "tel:") ||
			strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "#") {
			return
		}

		parsed, err := url.Parse(href)
		if err != nil {
			return
		}

		// Resolve relative to absolute
		resolved := base.ResolveReference(parsed)

		// Domain lock: only follow same-host links
		if resolved.Hostname() != allowedHost {
			return
		}

		// Skip non-HTML resource links
		if isResourceFile(resolved.Path) {
			return
		}

		// Strip fragment
		resolved.Fragment = ""

		canonical := resolved.String()
		if !seen[canonical] {
			seen[canonical] = true
			links = append(links, canonical)
		}
	})

	return links
}

// normalizeURL strips fragments and trailing slashes for deduplication.
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Fragment = ""
	result := u.String()
	// Remove trailing slash for consistency (except root "/")
	if len(result) > 1 && strings.HasSuffix(result, "/") {
		result = strings.TrimRight(result, "/")
	}
	return result
}

// isResourceFile returns true if the URL path points to a non-HTML resource.
func isResourceFile(urlPath string) bool {
	ext := strings.ToLower(path.Ext(urlPath))
	resourceExts := map[string]bool{
		".pdf": true, ".jpg": true, ".jpeg": true, ".png": true,
		".gif": true, ".svg": true, ".webp": true, ".ico": true,
		".mp4": true, ".mp3": true, ".avi": true, ".mov": true,
		".zip": true, ".tar": true, ".gz": true, ".rar": true,
		".css": true, ".js": true, ".woff": true, ".woff2": true,
		".ttf": true, ".eot": true, ".xml": true, ".rss": true,
		".json": true, ".txt": true,
	}
	return resourceExts[ext]
}
