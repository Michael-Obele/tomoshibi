package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

// RedisKV adapts *redis.Client to the KV interface, translating redis.Nil
// into kvNotFound.
type RedisKV struct {
	client *redis.Client
}

// NewRedisKV wraps a redis client as a KV store.
func NewRedisKV(client *redis.Client) *RedisKV {
	return &RedisKV{client: client}
}

func (k *RedisKV) Get(ctx context.Context, key string) (string, error) {
	val, err := k.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", errNotFound
	}
	return val, err
}

func (k *RedisKV) Set(ctx context.Context, key, value string) error {
	return k.client.Set(ctx, key, value, 0).Err()
}

func (k *RedisKV) Del(ctx context.Context, key string) error {
	return k.client.Del(ctx, key).Err()
}

// StartMonitorScheduler runs a background loop that enqueues monitor:check
// tasks when a monitor's NextCheck time arrives. Blocking; run in a
// goroutine. Stopped via ctx cancellation.
func StartMonitorScheduler(
	ctx context.Context,
	kv KV,
	enq Enqueuer,
	scraper *scraper.Service,
	logger *slog.Logger,
) {
	handler := NewMonitorTaskHandler(scraper, kv, logger)
	_ = handler // scheduler only enqueues; handler runs in the worker mux

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scanAndEnqueueDue(ctx, kv, enq, logger)
		}
	}
}

// scanAndEnqueueDue enqueues checks for all monitors whose NextCheck has
// passed. Redis keyscan is used; adapter types must expose scan.
func scanAndEnqueueDue(ctx context.Context, kv KV, enq Enqueuer, logger *slog.Logger) {
	scanner, ok := kv.(interface {
		Scan(ctx context.Context, pattern string, fn func(key string) bool) error
	})
	if !ok {
		return // non-redis KV in tests: nothing to schedule
	}

	now := time.Now()
	err := scanner.Scan(ctx, monitorPrefix+"*", func(key string) bool {
		if strings.HasSuffix(key, monitorHashSuffix) {
			return true // skip hash keys
		}
		id := strings.TrimPrefix(key, monitorPrefix)
		cfg, err := loadMonitorConfig(ctx, kv, id)
		if err != nil {
			return true
		}
		if !cfg.NextCheck.After(now) {
			task, err := NewMonitorCheckTask(id)
			if err != nil {
				logger.Error("Monitor task build failed", "monitor_id", id, "error", err)
				return true
			}
			if _, err := enq.Enqueue(task); err != nil {
				logger.Error("Monitor enqueue failed", "monitor_id", id, "error", err)
				return true
			}
		}
		return true
	})
	if err != nil {
		logger.Error("Monitor scan failed", "error", err)
	}
}

// redisScanKV extends RedisKV with SCAN support for the scheduler.
func (k *RedisKV) Scan(ctx context.Context, pattern string, fn func(key string) bool) error {
	iter := k.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if !fn(iter.Val()) {
			break
		}
	}
	return iter.Err()
}
