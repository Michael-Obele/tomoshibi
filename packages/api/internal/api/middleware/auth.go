// Package middleware provides Gin middlewares for authentication and
// rate limiting.
package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// APIKeyHeader is the header carrying the API key.
const APIKeyHeader = "X-API-Key"

// APIKeyAuth returns middleware requiring a valid X-API-Key header. When no
// keys are configured the API stays open (current default behavior).
func APIKeyAuth(keys []string) gin.HandlerFunc {
	valid := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k != "" {
			valid[k] = true
		}
	}
	enabled := len(valid) > 0

	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		key := c.GetHeader(APIKeyHeader)
		if !valid[key] {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing API key"})
			return
		}
		c.Next()
	}
}

// clientBucketKey is the Redis key for one client's minute window.
func clientBucketKey(client string) string {
	return "ratelimit:" + client + ":" + time.Now().UTC().Format("2006-01-02T15:04")
}

// redisLimiter enforces a per-minute cap per client using Redis INCR.
type redisLimiter struct {
	client *redis.Client
	rpm    int64
}

func (l *redisLimiter) Allow(key string) bool {
	return l.AllowWithContext(context.Background(), key)
}

// AllowWithContext is context-aware; callers with a request context should use it.
func (l *redisLimiter) AllowWithContext(ctx context.Context, key string) bool {
	n, err := l.client.Incr(ctx, clientBucketKey(key)).Result()
	if err != nil {
		return true // fail open on Redis errors; availability over strictness
	}
	if n == 1 {
		l.client.Expire(ctx, clientBucketKey(key), time.Minute)
	}
	return n <= l.rpm
}

// memLimiter is an in-memory fallback for deployments without Redis.
type memLimiter struct {
	mu     sync.Mutex
	window string
	counts map[string]int
	rpm    int64
}

func (m *memLimiter) Allow(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := time.Now().UTC().Format("2006-01-02T15:04")
	if w != m.window {
		m.window = w
		m.counts = make(map[string]int)
	}
	m.counts[key]++
	return int64(m.counts[key]) <= m.rpm
}

// RateLimit returns middleware limiting requests per client (API key, or
// client IP when open) to rpm per minute. Redis optional.
func RateLimit(rpm int, redisClient *redis.Client) gin.HandlerFunc {
	if rpm <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	type allowerWithContext interface {
		Allow(string) bool
		AllowWithContext(context.Context, string) bool
	}

	var lim interface{ Allow(string) bool }
	var limCtx allowerWithContext
	if redisClient != nil {
		rl := &redisLimiter{client: redisClient, rpm: int64(rpm)}
		lim = rl
		limCtx = rl
	} else {
		m := &memLimiter{rpm: int64(rpm)}
		lim = m
	}

	return func(c *gin.Context) {
		client := c.ClientIP()
		if key := c.GetHeader(APIKeyHeader); key != "" {
			client = key
		}
		allowed := false
		if limCtx != nil {
			allowed = limCtx.AllowWithContext(c.Request.Context(), client)
		} else {
			allowed = lim.Allow(client)
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": "60s",
			})
			return
		}
		c.Next()
	}
}
