package search

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSearchCacheKeyDeterministic(t *testing.T) {
	a := SearchOptions{Query: "golang", Limit: 10, IncludeDomains: []string{"go.dev"}}
	b := SearchOptions{Query: "golang", Limit: 10, IncludeDomains: []string{"go.dev"}}
	c := SearchOptions{Query: "golang", Limit: 10, IncludeDomains: []string{"github.com"}}

	if searchCacheKey(a) != searchCacheKey(b) {
		t.Error("identical options must produce identical cache keys")
	}
	if searchCacheKey(a) == searchCacheKey(c) {
		t.Error("different filters must produce different cache keys")
	}
}

// TestCachedServiceDoesNotCacheEmpty verifies a transient empty upstream
// response is NOT written to the cache, so it cannot become a sticky
// "no results" for a query that normally has plenty.
func TestCachedServiceDoesNotCacheEmpty(t *testing.T) {
	fc := newFakeCache()
	inner := &stubService{results: nil, count: 0} // always empty
	svc := &CachedService{inner: inner, cache: fc}

	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if fc.sets != 0 {
		t.Errorf("empty results were cached (%d writes); want 0", fc.sets)
	}
}

// TestCachedServiceCachesNonEmpty verifies non-empty results ARE cached and
// served on the next call without hitting the upstream service again.
func TestCachedServiceCachesNonEmpty(t *testing.T) {
	fc := newFakeCache()
	inner := &stubService{results: []Result{{Title: "hit", URL: "https://hit.dev/"}}, count: 1}
	svc := &CachedService{inner: inner, cache: fc}

	opts := SearchOptions{Query: "q", Limit: 10}
	if _, _, err := svc.Search(context.Background(), opts); err != nil {
		t.Fatalf("first Search returned error: %v", err)
	}
	if fc.sets != 1 {
		t.Errorf("expected 1 cache write, got %d", fc.sets)
	}

	// Second call must come from the cache, not the upstream stub.
	results, _, err := svc.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("second Search returned error: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("upstream called %d times; want 1 (second call should be cached)", inner.calls)
	}
	if len(results) != 1 || results[0].Title != "hit" {
		t.Errorf("cached results = %+v, want the hit", results)
	}
}

// fakeCache is an in-memory stand-in for the Redis search cache, mirroring
// the pattern used in internal/scraper.
type fakeCache struct {
	mu     sync.Mutex
	data   map[string]string
	sets   int
	getErr error
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
