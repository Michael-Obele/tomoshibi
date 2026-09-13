package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

// TypeMonitorCheck is the task type for scheduled monitor checks.
const TypeMonitorCheck = "monitor:check"

// monitorPrefix is the Redis key prefix for monitor configuration records.
const monitorPrefix = "monitor:"

// MonitorConfig is the persisted state of a change-tracking monitor.
type MonitorConfig struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	IntervalSeconds int       `json:"interval_seconds"`
	WebhookURL      string    `json:"webhook_url,omitempty"`
	WebhookSecret   string    `json:"webhook_secret,omitempty"`
	NextCheck       time.Time `json:"next_check"`
}

// monitorHashSuffix is appended to the config key for the last content hash.
const monitorHashSuffix = ":hash"

// KV is the minimal key-value surface the monitor needs (satisfied by
// *redis.Client; implemented in-memory for tests).
type KV interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	Del(ctx context.Context, key string) error
}

// Enqueuer is the Asynq task-submission surface (satisfied by *asynq.Client).
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// MonitorPayload identifies which monitor to check.
type MonitorPayload struct {
	ID string `json:"id"`
}

// NewMonitorCheckTask creates a monitor check task.
func NewMonitorCheckTask(id string) (*asynq.Task, error) {
	data, err := json.Marshal(MonitorPayload{ID: id})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal monitor payload: %w", err)
	}
	return asynq.NewTask(TypeMonitorCheck, data, asynq.Retention(24*time.Hour)), nil
}

// MonitorTaskHandler processes monitor:check tasks: scrape, hash, compare,
// fire webhook on change.
type MonitorTaskHandler struct {
	scraper *scraper.Service
	kv      KV
	logger  *slog.Logger
}

// NewMonitorTaskHandler constructs a MonitorTaskHandler.
func NewMonitorTaskHandler(scraper *scraper.Service, kv KV, logger *slog.Logger) *MonitorTaskHandler {
	return &MonitorTaskHandler{scraper: scraper, kv: kv, logger: logger}
}

// ProcessTask is the Asynq entry point for monitor checks.
func (h *MonitorTaskHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload MonitorPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal monitor payload: %w", err)
	}
	changed, err := h.checkForChange(ctx, payload.ID)
	if err != nil {
		return err
	}
	if changed {
		h.logger.Info("Monitor detected change", "monitor_id", payload.ID)
	}
	return nil
}

// checkForChange scrapes the monitored URL, compares the markdown hash
// against the last stored hash, and fires the webhook when content changed.
// The first check always stores the baseline without firing.
func (h *MonitorTaskHandler) checkForChange(ctx context.Context, id string) (bool, error) {
	cfg, err := loadMonitorConfig(ctx, h.kv, id)
	if err != nil {
		return false, err
	}

	result, err := h.scraper.Scrape(ctx, cfg.URL, "smart", domain.ScrapeOptions{})
	if err != nil {
		return false, fmt.Errorf("monitor scrape failed: %w", err)
	}

	newHash := contentHash(result.Markdown)
	hashKey := monitorPrefix + id + monitorHashSuffix

	oldHash, getErr := h.kv.Get(ctx, hashKey)
	if getErr != nil && getErr != errNotFound {
		return false, fmt.Errorf("monitor hash read failed: %w", getErr)
	}

	if getErr == nil && oldHash == newHash {
		return false, h.reschedule(ctx, cfg)
	}

	if err := h.kv.Set(ctx, hashKey, newHash); err != nil {
		return false, fmt.Errorf("monitor hash store failed: %w", err)
	}

	// Baseline (first run) → store, no notification.
	if getErr == errNotFound {
		return false, h.reschedule(ctx, cfg)
	}

	if cfg.WebhookURL != "" {
		payload, _ := json.Marshal(map[string]any{
			"monitor_id": id,
			"url":        cfg.URL,
			"changed":    true,
			"hash_old":   oldHash,
			"hash_new":   newHash,
			"changed_at": time.Now().UTC().Format(time.RFC3339),
		})
		if err := Deliver(ctx, cfg.WebhookURL, cfg.WebhookSecret, payload); err != nil {
			h.logger.Warn("Monitor webhook delivery failed", "monitor_id", id, "error", err)
		}
	}

	return true, h.reschedule(ctx, cfg)
}

// reschedule advances NextCheck by the configured interval.
func (h *MonitorTaskHandler) reschedule(ctx context.Context, cfg *MonitorConfig) error {
	cfg.NextCheck = time.Now().Add(time.Duration(cfg.IntervalSeconds) * time.Second)
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return h.kv.Set(ctx, monitorPrefix+cfg.ID, string(data))
}

// errNotFound is returned by Get when a key is absent.
var errNotFound = fmt.Errorf("key not found")

// contentHash returns the SHA-256 hex digest of the markdown content.
func contentHash(markdown string) string {
	sum := sha256.Sum256([]byte(markdown))
	return hex.EncodeToString(sum[:])
}

// loadMonitorConfig reads and validates a monitor's persisted config.
func loadMonitorConfig(ctx context.Context, kv KV, id string) (*MonitorConfig, error) {
	raw, err := kv.Get(ctx, monitorPrefix+id)
	if err != nil {
		return nil, fmt.Errorf("monitor %q not found: %w", id, err)
	}
	var cfg MonitorConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("corrupt monitor config %q: %w", id, err)
	}
	return &cfg, nil
}
