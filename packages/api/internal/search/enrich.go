package search

import (
	"context"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// Search-time content composition.
//
// A search returns titles, URLs and snippets; an LLM caller then needs the
// page body, which today means a second round-trip per result. This closes
// that gap: the same call that finds the results also fetches them.
//
// The design rule is that enrichment is strictly additive. A URL that fails
// to fetch keeps its SERP row with empty Content, because a title and a
// description are still worth returning and silently dropping rows would
// change the count/hasMore contract callers already depend on.

// defaultEnrichConcurrency matches the multi-scrape limit used by the scrape
// handler, so composition cannot open more browser tabs than an explicit
// batch scrape already does.
const defaultEnrichConcurrency = 5

// ContentFetcher retrieves the page body for one URL. It is declared here, by
// the consumer, so this package never imports the scraper — *scraper.Service
// satisfies it structurally.
type ContentFetcher interface {
	Scrape(ctx context.Context, url string, mode string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error)
}

// EnrichOptions controls composition. The zero value enriches nothing, so a
// caller must opt in explicitly.
type EnrichOptions struct {
	// Limit caps how many of the top results are fetched. 0 means all.
	Limit int
	// Mode is the scrape mode ("smart", "static", "dynamic"); "" lets the
	// scraper pick its default.
	Mode string
	// Concurrency bounds simultaneous fetches; <= 0 uses the default.
	Concurrency int
}

// EnrichResults fetches page content for the leading results and returns the
// same slice with Content filled in. It is a no-op when fetch is nil or the
// options request nothing.
//
// Results are written at their own index, so the slice is safe to mutate
// concurrently; the caller's ordering is never disturbed.
func EnrichResults(ctx context.Context, results []Result, fetch ContentFetcher, opts EnrichOptions) []Result {
	if fetch == nil || len(results) == 0 {
		return results
	}
	n := opts.Limit
	if n <= 0 || n > len(results) {
		n = len(results)
	}
	limit := opts.Concurrency
	if limit <= 0 {
		limit = defaultEnrichConcurrency
	}

	// Per-index outcome flags rather than shared counters: each goroutine
	// writes only its own slot, so the tally needs no atomics and stays
	// race-free under -race.
	ok := make([]bool, n)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for i := 0; i < n; i++ {
		i := i
		url := strings.TrimSpace(results[i].URL)
		if url == "" {
			continue
		}
		g.Go(func() error {
			res, err := fetch.Scrape(gctx, url, opts.Mode, domain.ScrapeOptions{})
			if err != nil {
				// A cancelled parent means the caller went away; stop the
				// remaining work rather than burning fetches on it.
				if gctx.Err() != nil {
					return gctx.Err()
				}
				if logger.Log != nil {
					logger.Log.Warn("search: content fetch failed", "url", url, "error", err)
				}
				return nil
			}
			if res != nil {
				results[i].Content = res.Markdown
				ok[i] = true
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil && ctx.Err() == nil && logger.Log != nil {
		// Only reachable when the parent context is still live, i.e. an
		// unexpected internal failure rather than a client disconnect.
		logger.Log.Warn("search: content composition aborted", "error", err)
	}
	fetched := 0
	for _, good := range ok {
		if good {
			fetched++
		}
	}
	if logger.Log != nil && fetched > 0 {
		logger.Log.Info("search: content composition done",
			"requested", n, "fetched", fetched, "failed", n-fetched)
	}
	return results
}
