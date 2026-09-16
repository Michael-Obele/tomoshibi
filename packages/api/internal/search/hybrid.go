package search

import (
	"context"
	"errors"
	"fmt"

	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// HybridService tries a chain of search backends in order and returns the
// first non-empty result set. Backends are ordered cheap-to-robust:
// SearXNG (self-hosted, free) → Stealth (browser-rendered Brave HTML via
// shared Chromedp allocator, free but CPU) → Brave API (1 QPS, ~2000/mo
// free, metered last resort). A backend that errors or returns nothing falls
// through to the next, so a single engine outage never kills search.
type HybridService struct {
	services []Service
}

// NewHybridService builds the search backend chain from configuration without
// stealth. It delegates to NewHybridServiceWithStealth with a nil fetcher, so
// stealth is disabled by default. See NewHybridServiceWithStealth for the
// full 3-backend chain.
//
//   - braveAPIKey: optional Brave Search API key. When set it is the
//     fallback when SearXNG is unavailable or unconfigured.
//   - searxngEndpoint: self-hosted SearXNG base URL. When set it is the
//     primary backend — free, aggregates many engines (Google, Bing,
//     DuckDuckGo, Brave, Mojeek, Wikipedia, ...), and has no per-query cost.
//
// When neither is configured, a Service is returned that fails with a clear
// message so a misconfigured deployment fails loudly instead of silently
// returning empty results.
func NewHybridService(braveAPIKey, searxngEndpoint string) Service {
	return NewHybridServiceWithStealth(braveAPIKey, searxngEndpoint, nil)
}

// NewHybridServiceWithStealth builds the full 3-backend search chain
// SearXNG → Stealth → Brave API (cheap → free fallback → metered last resort).
//
// Ordering is intentional: SearXNG is cheapest (self-hosted, no per-query
// cost, aggregates many engines), Stealth is free but CPU-heavy
// (browser-rendered Brave HTML via shared ChromedpScraper), and Brave API
// is metered (1 QPS, ~2000/mo) so it is last to conserve quota. The chain
// falls through on error or empty results, so a SearXNG 429/captcha tries
// Stealth before spending Brave API quota.
//
//   - braveAPIKey: optional Brave Search API key (last resort).
//   - searxngEndpoint: optional self-hosted SearXNG base URL.
//   - fetcher: optional BrowserFetcher (typically *scraper.ChromedpScraper).
//     When nil, stealth is disabled and the chain is SearXNG → Brave only.
//     When non-nil, Stealth is inserted before Brave API.
//
// When no backends are configured, a Service is returned that fails with a
// clear configuration error.
func NewHybridServiceWithStealth(braveAPIKey, searxngEndpoint string, fetcher BrowserFetcher) Service {
	chain := make([]Service, 0, 3)
	if searxngEndpoint != "" {
		chain = append(chain, NewSearXNGService(searxngEndpoint))
	}
	if fetcher != nil {
		chain = append(chain, NewStealthService(fetcher, ""))
	}
	if braveAPIKey != "" {
		chain = append(chain, NewBraveService(braveAPIKey))
	}

	switch len(chain) {
	case 0:
		return noBackendService{}
	case 1:
		return chain[0]
	default:
		return &HybridService{services: chain}
	}
}

func (h *HybridService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
	var lastErr error
	for _, s := range h.services {
		results, total, err := s.Search(ctx, opts)
		if err == nil && len(results) > 0 {
			return results, total, nil
		}
		if err != nil {
			lastErr = err
			if logger.Log != nil {
				logger.Log.Info("search: backend failed, trying next", "backend", fmt.Sprintf("%T", s), "error", err)
			}
		} else if logger.Log != nil {
			logger.Log.Info("search: backend returned empty, trying next", "backend", fmt.Sprintf("%T", s))
		}
	}
	if lastErr != nil {
		return nil, 0, lastErr
	}
	return nil, 0, nil
}

// noBackendService fails every search with a configuration error. It is
// returned when neither SearXNG nor Brave is configured, so operators see
// exactly what to set instead of an empty result set.
type noBackendService struct{}

func (noBackendService) Search(context.Context, SearchOptions) ([]Result, int, error) {
	return nil, 0, errors.New("search is not configured: set SEARXNG_ENDPOINT (recommended) or BRAVE_SEARCH_API_KEY")
}
