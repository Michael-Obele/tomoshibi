package scraper

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/redis/go-redis/v9"
)

// fakeCache is an in-memory stand-in for the Redis scrape cache.
//
// It exists because every cache branch in Scrape — the gzip read, the legacy
// uncompressed fallback, the unusable-value fall-through, and the write — was
// unreachable from tests while the field was a concrete *redis.Client. Values
// are stored as strings because that is what Redis hands back on Get.
type fakeCache struct {
	mu      sync.Mutex
	data    map[string]string
	getErr  error
	sets    int
	lastTTL time.Duration
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: make(map[string]string)}
}

func (f *fakeCache) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx, "get", key)

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getErr != nil {
		cmd.SetErr(f.getErr)
		return cmd
	}
	val, ok := f.data[key]
	if !ok {
		// redis.Nil is the miss sentinel a real client returns, and the
		// production code treats any error as "no usable cache entry".
		cmd.SetErr(redis.Nil)
		return cmd
	}
	cmd.SetVal(val)
	return cmd
}

func (f *fakeCache) Set(ctx context.Context, key string, value any, ttl time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx, "set", key)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets++
	f.lastTTL = ttl

	switch v := value.(type) {
	case []byte:
		f.data[key] = string(v)
	case string:
		f.data[key] = v
	default:
		cmd.SetErr(fmt.Errorf("fakeCache: unexpected value type %T", value))
		return cmd
	}
	cmd.SetVal("OK")
	return cmd
}

// withCache builds a Service backed by an in-memory cache. NewService only
// accepts a concrete *redis.Client, so the field is set directly.
func withCache(colly, chromedp domain.Scraper, c *fakeCache) *Service {
	s := NewService(colly, chromedp, nil)
	s.cache = c
	return s
}

// gzipped compresses v exactly the way the cache write path does, so stored
// fixtures are byte-identical to what production writes.
func gzipped(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return b.String()
}

// unusableScraper fails the test if it is ever called. Cache-hit tests use it
// to prove the hit short-circuits the engines, rather than merely returning
// content that happens to match.
type unusableScraper struct{ t *testing.T }

func (u *unusableScraper) Scrape(context.Context, string, domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	u.t.Helper()
	u.t.Error("an engine was invoked despite a cache hit")
	return nil, errors.New("should not be called")
}

func TestScrape_CacheHit(t *testing.T) {
	const url = "https://cached.example.com"

	tests := []struct {
		name   string
		stored domain.ScrapeResult
	}{
		{
			name: "gzipped entry",
			stored: domain.ScrapeResult{
				URL:      url,
				Markdown: "# From cache",
				Metadata: map[string]string{"engine": "colly"},
			},
		},
		{
			// Entries written before compression was introduced are still
			// live in Redis for up to the 7-day TTL.
			name: "nil metadata still gets the cached marker",
			stored: domain.ScrapeResult{
				URL:      url,
				Markdown: "# From cache",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newFakeCache()
			key := cacheKeyFor(url, "static", domain.ScrapeOptions{})
			cache.data[key] = gzipped(t, tt.stored)

			svc := withCache(&unusableScraper{t}, &unusableScraper{t}, cache)

			result, err := svc.Scrape(context.Background(), url, "static", domain.ScrapeOptions{})
			if err != nil {
				t.Fatalf("Scrape returned an error: %v", err)
			}
			if result.Markdown != tt.stored.Markdown {
				t.Errorf("Markdown = %q, want %q", result.Markdown, tt.stored.Markdown)
			}
			if result.Metadata["cached"] != "true" {
				t.Errorf(`Metadata["cached"] = %q, want "true"`, result.Metadata["cached"])
			}
			if cache.sets != 0 {
				t.Errorf("a cache hit rewrote the entry %d time(s)", cache.sets)
			}
		})
	}
}

// TestScrape_CacheHitLegacyUncompressed covers the fallback for entries
// written before the values were gzipped. They are plain JSON, so
// gzip.NewReader rejects them and the read path must unmarshal directly.
func TestScrape_CacheHitLegacyUncompressed(t *testing.T) {
	const url = "https://legacy.example.com"

	stored := domain.ScrapeResult{URL: url, Markdown: "# Legacy"}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	cache := newFakeCache()
	cache.data[cacheKeyFor(url, "static", domain.ScrapeOptions{})] = string(raw)

	svc := withCache(&unusableScraper{t}, &unusableScraper{t}, cache)

	result, err := svc.Scrape(context.Background(), url, "static", domain.ScrapeOptions{})
	if err != nil {
		t.Fatalf("Scrape returned an error: %v", err)
	}
	if result.Markdown != "# Legacy" {
		t.Errorf("Markdown = %q, want %q", result.Markdown, "# Legacy")
	}
	if result.Metadata["cached"] != "true" {
		t.Errorf(`Metadata["cached"] = %q, want "true"`, result.Metadata["cached"])
	}
}

// TestScrape_CacheFallsThroughOnUnusableValue asserts a damaged or
// unreadable cache entry degrades to a live scrape instead of failing the
// request. A cache is an optimisation; it must never be able to break a
// scrape that would otherwise succeed.
func TestScrape_CacheFallsThroughOnUnusableValue(t *testing.T) {
	const url = "https://example.com"
	key := cacheKeyFor(url, "static", domain.ScrapeOptions{})

	tests := []struct {
		name    string
		prepare func(*fakeCache)
	}{
		{
			name: "value is neither gzip nor json",
			prepare: func(c *fakeCache) {
				c.data[key] = "\x00not gzip, not json"
			},
		},
		{
			name: "gzip wrapping invalid json",
			prepare: func(c *fakeCache) {
				var b bytes.Buffer
				gz := gzip.NewWriter(&b)
				_, _ = gz.Write([]byte("{ not json"))
				_ = gz.Close()
				c.data[key] = b.String()
			},
		},
		{
			name: "the get itself fails",
			prepare: func(c *fakeCache) {
				c.getErr = errors.New("connection reset by peer")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newFakeCache()
			tt.prepare(cache)

			colly := &mockScraper{result: newMockResult("colly")}
			svc := withCache(colly, nil, cache)

			result, err := svc.Scrape(context.Background(), url, "static", domain.ScrapeOptions{})
			if err != nil {
				t.Fatalf("Scrape returned an error: %v", err)
			}
			if result.Metadata["engine"] != "colly" {
				t.Errorf("engine = %q, want the live scrape result", result.Metadata["engine"])
			}
			if result.Metadata["cached"] != "" {
				t.Error("a fall-through result was marked as cached")
			}
			if cache.sets != 1 {
				t.Errorf("cache writes = %d, want 1 (the fresh result should be stored)", cache.sets)
			}
		})
	}
}

// TestScrape_CacheRoundTrip covers the write path and then reads what it
// wrote back through the read path. Asserting the two halves agree is what
// catches a compression or key mismatch; checking either one alone does not.
func TestScrape_CacheRoundTrip(t *testing.T) {
	const url = "https://roundtrip.example.com"
	opts := domain.ScrapeOptions{}

	cache := newFakeCache()
	colly := &mockScraper{result: newMockResult("colly")}
	svc := withCache(colly, nil, cache)

	first, err := svc.Scrape(context.Background(), url, "static", opts)
	if err != nil {
		t.Fatalf("first Scrape returned an error: %v", err)
	}
	if first.Metadata["cached"] != "" {
		t.Error("the first scrape was served from an empty cache")
	}

	if cache.sets != 1 {
		t.Fatalf("cache writes = %d, want 1", cache.sets)
	}
	if want := 7 * 24 * time.Hour; cache.lastTTL != want {
		t.Errorf("TTL = %v, want %v", cache.lastTTL, want)
	}
	if _, ok := cache.data[cacheKeyFor(url, "static", opts)]; !ok {
		t.Error("the entry was not written under cacheKeyFor's key")
	}

	// Second call: same URL, mode and options, so it must hit.
	svc = withCache(&unusableScraper{t}, &unusableScraper{t}, cache)
	second, err := svc.Scrape(context.Background(), url, "static", opts)
	if err != nil {
		t.Fatalf("second Scrape returned an error: %v", err)
	}
	if second.Metadata["cached"] != "true" {
		t.Errorf(`Metadata["cached"] = %q, want "true"`, second.Metadata["cached"])
	}
	if second.Markdown != first.Markdown {
		t.Errorf("Markdown = %q, want %q", second.Markdown, first.Markdown)
	}
}

// TestScrape_CacheMissesOnChangedOptions is the behavioural half of
// TestCacheKeyFor_DiffersOnOptions: a different option set must reach the
// engine again rather than serving the previous answer.
func TestScrape_CacheMissesOnChangedOptions(t *testing.T) {
	const url = "https://opts.example.com"

	cache := newFakeCache()
	colly := &mockScraper{result: newMockResult("colly")}
	svc := withCache(colly, nil, cache)

	if _, err := svc.Scrape(context.Background(), url, "static", domain.ScrapeOptions{Summary: true}); err != nil {
		t.Fatalf("first Scrape returned an error: %v", err)
	}
	if _, err := svc.Scrape(context.Background(), url, "static", domain.ScrapeOptions{Summary: false}); err != nil {
		t.Fatalf("second Scrape returned an error: %v", err)
	}

	if cache.sets != 2 {
		t.Errorf("cache writes = %d, want 2 — the option change should have missed", cache.sets)
	}
	if len(cache.data) != 2 {
		t.Errorf("cache holds %d entries, want 2 distinct keys", len(cache.data))
	}
}
