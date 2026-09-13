package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// searchCacheTTL bounds how long search results are cached. SearXNG
// aggregates many engines and is fast; caching repeat queries turns an
// upstream round-trip into a Redis read and keeps the engine healthy.
const searchCacheTTL = 5 * time.Minute

// cachedPayload is the JSON shape stored under a search cache key.
type cachedPayload struct {
	Results []Result `json:"results"`
	Total   int      `json:"total"`
}

// cacheStore is the slice of the Redis client the search cache uses.
// Narrowing it to these two methods lets tests drive the cache with an
// in-memory fake instead of a live server; *redis.Client satisfies it.
type cacheStore interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
}

// CachedService wraps a Service with a Redis-backed result cache. When Redis
// is unavailable the cache degrades to a pass-through; a cache miss or write
// failure never fails the request.
type CachedService struct {
	inner Service
	cache cacheStore
}

// NewCachedService wraps inner with a Redis-backed result cache. A nil rdb
// returns inner unchanged, so callers can pass the client unconditionally.
func NewCachedService(inner Service, rdb *redis.Client) Service {
	if rdb == nil {
		return inner
	}
	return &CachedService{inner: inner, cache: rdb}
}

// Search returns cached results when available, otherwise performs the
// upstream search and stores the outcome for subsequent callers.
func (c *CachedService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
	key := searchCacheKey(opts)

	if val, err := c.cache.Get(ctx, key).Result(); err == nil {
		var cached cachedPayload
		if json.Unmarshal([]byte(val), &cached) == nil {
			return cached.Results, cached.Total, nil
		}
	}

	results, total, err := c.inner.Search(ctx, opts)
	if err != nil {
		return nil, 0, err
	}

	// Only cache non-empty result sets. A transient empty response from an
	// upstream engine must not become a sticky 5-minute "no results" for a
	// query that normally has plenty — SearXNG occasionally returns nothing
	// when all its engines hiccup at once.
	if len(results) > 0 {
		data, err := json.Marshal(cachedPayload{Results: results, Total: total})
		if err != nil {
			return results, total, nil
		}
		if c.cache.Set(ctx, key, data, searchCacheTTL).Err() != nil {
			// Non-fatal: a failed cache write must not fail the search.
		}
	}
	return results, total, nil
}

// searchCacheKey derives a deterministic key from the full option set so
// distinct filters never share a cache entry.
func searchCacheKey(opts SearchOptions) string {
	payload := struct {
		Query          string   `json:"q"`
		Offset         int      `json:"o"`
		Limit          int      `json:"l"`
		IncludeDomains []string `json:"inc,omitempty"`
		ExcludeDomains []string `json:"exc,omitempty"`
		RequiredText   []string `json:"req,omitempty"`
		MaxAge         *int     `json:"age,omitempty"`
		Mode           string   `json:"mode,omitempty"`
		Category       string   `json:"cat,omitempty"`
		Rerank         bool     `json:"rerank,omitempty"`
	}{
		Query:          opts.Query,
		Offset:         opts.Offset,
		Limit:          opts.Limit,
		IncludeDomains: opts.IncludeDomains,
		ExcludeDomains: opts.ExcludeDomains,
		RequiredText:   opts.RequiredText,
		MaxAge:         opts.MaxAge,
		Mode:           opts.Mode,
		Category:       opts.Category,
		Rerank:         opts.Rerank,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		// Marshal of these types cannot fail; degrade to a query-only key.
		return fmt.Sprintf("search:%s", opts.Query)
	}
	sum := sha256.Sum256(data)
	return "search:" + hex.EncodeToString(sum[:])
}
