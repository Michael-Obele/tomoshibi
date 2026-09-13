package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

// memKV is an in-memory KV store for tests.
type memKV struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemKV() *memKV {
	return &memKV{data: make(map[string]string)}
}

func (m *memKV) Get(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return "", errNotFound
}

func (m *memKV) Set(ctx context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *memKV) Del(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// fakeEnqueuer records enqueued task payloads.
type fakeEnqueuer struct {
	mu       sync.Mutex
	enqueued []string
}

func (f *fakeEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueued = append(f.enqueued, string(task.Payload()))
	return &asynq.TaskInfo{ID: fmt.Sprintf("t%d", len(f.enqueued))}, nil
}

// mutableScraper returns a fixed markdown until changed.
type mutableScraper struct {
	mu       sync.Mutex
	markdown string
}

func (m *mutableScraper) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &domain.ScrapeResult{
		URL:      url,
		Markdown: m.markdown,
		HTML:     "<html><body>" + m.markdown + "</body></html>",
		Metadata: map[string]string{"engine": "mock"},
	}, nil
}

// TestMonitorCheck_BaselineStoresHash verifies the first check stores a
// baseline without firing a webhook.
func TestMonitorCheck_BaselineStoresHash(t *testing.T) {
	kv := newMemKV()
	cfg := MonitorConfig{ID: "m1", URL: "https://x.example.com", IntervalSeconds: 3600}
	data, _ := json.Marshal(cfg)
	_ = kv.Set(context.Background(), monitorPrefix+"m1", string(data))

	s := &mutableScraper{markdown: "# v1"}
	handler := NewMonitorTaskHandler(scraper.NewService(s, nil, nil), kv, newTestLogger())

	changed, err := handler.checkForChange(context.Background(), "m1")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if changed {
		t.Error("baseline must not report change")
	}
	hash, err := kv.Get(context.Background(), monitorPrefix+"m1"+monitorHashSuffix)
	if err != nil {
		t.Fatalf("baseline hash not stored: %v", err)
	}
	if hash != contentHash("# v1") {
		t.Errorf("unexpected stored hash %q", hash)
	}
}

// TestMonitorCheck_DetectsChange verifies modified content fires a webhook
// and updates the stored hash.
func TestMonitorCheck_DetectsChange(t *testing.T) {
	kv := newMemKV()
	cfg := MonitorConfig{ID: "m1", URL: "https://x.example.com", IntervalSeconds: 3600, WebhookURL: "http://localhost:1/wh", WebhookSecret: "s"}
	data, _ := json.Marshal(cfg)
	_ = kv.Set(context.Background(), monitorPrefix+"m1", string(data))
	_ = kv.Set(context.Background(), monitorPrefix+"m1"+monitorHashSuffix, contentHash("# v1"))

	s := &mutableScraper{markdown: "# v2 changed"}
	handler := NewMonitorTaskHandler(scraper.NewService(s, nil, nil), kv, newTestLogger())

	changed, err := handler.checkForChange(context.Background(), "m1")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if !changed {
		t.Error("change must be reported")
	}
	hash, _ := kv.Get(context.Background(), monitorPrefix+"m1"+monitorHashSuffix)
	if hash != contentHash("# v2 changed") {
		t.Errorf("hash not updated: %q", hash)
	}
}

// TestMonitorCheck_UnchangedContent verifies identical content is a no-op.
func TestMonitorCheck_UnchangedContent(t *testing.T) {
	kv := newMemKV()
	cfg := MonitorConfig{ID: "m1", URL: "https://x.example.com", IntervalSeconds: 3600}
	data, _ := json.Marshal(cfg)
	_ = kv.Set(context.Background(), monitorPrefix+"m1", string(data))
	_ = kv.Set(context.Background(), monitorPrefix+"m1"+monitorHashSuffix, contentHash("# same"))

	s := &mutableScraper{markdown: "# same"}
	handler := NewMonitorTaskHandler(scraper.NewService(s, nil, nil), kv, newTestLogger())

	changed, err := handler.checkForChange(context.Background(), "m1")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if changed {
		t.Error("unchanged content must not report change")
	}
}

// TestScanAndEnqueueDue verifies the scheduler enqueues only due monitors.
func TestScanAndEnqueueDue(t *testing.T) {
	kv := &scanMemKV{memKV: newMemKV()}
	due := MonitorConfig{ID: "due", URL: "https://a.example.com", IntervalSeconds: 3600, NextCheck: time.Now().Add(-time.Minute)}
	soon := MonitorConfig{ID: "soon", URL: "https://b.example.com", IntervalSeconds: 3600, NextCheck: time.Now().Add(time.Hour)}
	du, _ := json.Marshal(due)
	so, _ := json.Marshal(soon)
	_ = kv.Set(context.Background(), monitorPrefix+"due", string(du))
	_ = kv.Set(context.Background(), monitorPrefix+"soon", string(so))

	enq := &fakeEnqueuer{}
	scanAndEnqueueDue(context.Background(), kv, enq, newTestLogger())

	enq.mu.Lock()
	defer enq.mu.Unlock()
	if len(enq.enqueued) != 1 {
		t.Fatalf("expected 1 enqueue, got %d: %v", len(enq.enqueued), enq.enqueued)
	}
	if !strings.Contains(enq.enqueued[0], "due") {
		t.Errorf("expected due monitor payload, got %s", enq.enqueued[0])
	}
}

// scanMemKV wraps memKV with Scan support backed by its key map.
type scanMemKV struct {
	*memKV
}

func (s *scanMemKV) Scan(ctx context.Context, pattern string, fn func(key string) bool) error {
	prefix := strings.TrimSuffix(pattern, "*")
	// Collect keys under the lock, then invoke the callback outside it:
	// the callback performs KV reads that need the same mutex.
	s.mu.Lock()
	var keys []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	s.mu.Unlock()

	for _, k := range keys {
		if !fn(k) {
			break
		}
	}
	return nil
}
