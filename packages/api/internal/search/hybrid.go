package search

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/telemetry"
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
	// rec records one telemetry event per search (nil = recording off).
	rec telemetry.Recorder
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
	return NewHybridServiceWithNative(nil, nil, braveAPIKey, searxngEndpoint, fetcher)
}

// NewHybridServiceWithNative builds the full chain with the in-house
// native engine layer first: Native → SearXNG → Stealth → Brave API.
// The native layer is cheapest and has no sidecar dependency, so it leads;
// a nil native restores the legacy ordering exactly. Plan M5 later removes
// SearXNG from this chain after the parity gate.
func NewHybridServiceWithNative(rec telemetry.Recorder, native Service, braveAPIKey, searxngEndpoint string, fetcher BrowserFetcher) Service {
	if rec == nil {
		rec = telemetry.Nop{}
	}
	chain := make([]Service, 0, 4)
	if native != nil {
		chain = append(chain, native)
	}
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
		return &HybridService{services: chain, rec: rec}
	}
}

// Search walks the chain and returns the first strong result set. A set that
// is merely non-empty but concentrated on a single domain (see isWeak) does
// not end the walk: it is remembered and returned only when every following
// backend fails or answers empty, so concentrated junk never masks a better
// fallback while a weak-but-usable set is never discarded for nothing.
// Every call emits one telemetry.SearchEvent (trace id joins the native
// layer's per-engine events).
func (h *HybridService) Search(ctx context.Context, opts SearchOptions) (results []Result, total int, err error) {
	start := time.Now()
	ctx = telemetry.WithTraceID(ctx, telemetry.NewTraceID())

	var (
		lastErr     error
		weakResults []Result
		weakTotal   int
		weakFrom    string
		fallbacks   []string
	)
	answered := "none"
	defer func() {
		if h.rec == nil {
			return
		}
		h.rec.RecordSearch(telemetry.SearchEvent{
			TraceID:   telemetry.TraceID(ctx),
			Query:     opts.Query,
			Category:  opts.Category,
			Backend:   answered,
			Fallbacks: fallbacks,
			Engines:   engineNames(results),
			Results:   len(results),
			LatencyMS: time.Since(start).Milliseconds(),
			Weak:      weakFrom != "",
			Error:     errText(err),
		})
	}()

	for i, s := range h.services {
		res, tot, e := s.Search(ctx, opts)
		if e == nil && len(res) > 0 {
			if !isWeak(res) {
				answered = backendName(s)
				return res, tot, nil
			}
			if weakResults == nil {
				weakResults, weakTotal = res, tot
				weakFrom = backendName(s)
				fallbacks = append(fallbacks, weakFrom+": weak")
				if logger.Log != nil {
					logger.Log.Info("search: results concentrated on one domain, trying next",
						"backend", weakFrom,
						"results", len(res),
						"last_backend", i == len(h.services)-1)
				}
			}
			continue
		}
		if e != nil {
			lastErr = e
			fallbacks = append(fallbacks, backendName(s)+": error: "+e.Error())
			if logger.Log != nil {
				logger.Log.Info("search: backend failed, trying next", "backend", fmt.Sprintf("%T", s), "error", e)
			}
		} else {
			fallbacks = append(fallbacks, backendName(s)+": empty")
			if logger.Log != nil {
				logger.Log.Info("search: backend returned empty, trying next", "backend", fmt.Sprintf("%T", s))
			}
		}
	}
	if weakResults != nil {
		answered = weakFrom
		return weakResults, weakTotal, nil // nothing stronger answered
	}
	if lastErr != nil {
		return nil, 0, lastErr
	}
	return nil, 0, nil
}

// backendName maps a chain member to the short name used in logs, telemetry
// and /v1/insights.
func backendName(s Service) string {
	switch s.(type) {
	case *NativeService:
		return "native"
	case *SearXNGService:
		return "searxng"
	case *StealthService:
		return "stealth"
	case *BraveService:
		return "brave"
	default:
		return fmt.Sprintf("%T", s)
	}
}

// engineNames lists the distinct native engines that contributed results.
func engineNames(results []Result) []string {
	var seen []string
	have := map[string]bool{}
	for _, r := range results {
		if r.Engine != "" && !have[r.Engine] {
			have[r.Engine] = true
			seen = append(seen, r.Engine)
		}
	}
	return seen
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// noBackendService fails every search with a configuration error. It is
// returned when neither SearXNG nor Brave is configured, so operators see
// exactly what to set instead of an empty result set.
type noBackendService struct{}

func (noBackendService) Search(context.Context, SearchOptions) ([]Result, int, error) {
	return nil, 0, errors.New("search is not configured: set SEARXNG_ENDPOINT (recommended) or BRAVE_SEARCH_API_KEY")
}
