package search

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// fakeFetcher records calls and returns canned markdown, optionally failing or
// stalling for specific URLs.
type fakeFetcher struct {
	mu       sync.Mutex
	calls    []string
	inflight int32
	peak     int32
	fail     map[string]error
	delay    time.Duration
}

func (f *fakeFetcher) Scrape(ctx context.Context, url, mode string, _ domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	cur := atomic.AddInt32(&f.inflight, 1)
	for {
		peak := atomic.LoadInt32(&f.peak)
		if cur <= peak || atomic.CompareAndSwapInt32(&f.peak, peak, cur) {
			break
		}
	}
	defer atomic.AddInt32(&f.inflight, -1)

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, url)
	f.mu.Unlock()
	if err, ok := f.fail[url]; ok {
		return nil, err
	}
	return &domain.ScrapeResult{URL: url, Markdown: "# " + url}, nil
}

func (f *fakeFetcher) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func mkResults(n int) []Result {
	out := make([]Result, n)
	for i := range out {
		out[i] = Result{
			Title:  "t",
			URL:    "https://example.com/" + string(rune('a'+i)),
			Domain: "example.com",
		}
	}
	return out
}

func TestEnrichResultsFillsContent(t *testing.T) {
	f := &fakeFetcher{}
	res := EnrichResults(context.Background(), mkResults(3), f, EnrichOptions{})

	if len(res) != 3 {
		t.Fatalf("len = %d, want 3", len(res))
	}
	for i, r := range res {
		if r.Content == "" {
			t.Errorf("result %d has empty content", i)
		}
	}
	if got := len(f.called()); got != 3 {
		t.Errorf("fetched %d urls, want 3", got)
	}
}

func TestEnrichResultsLimit(t *testing.T) {
	f := &fakeFetcher{}
	res := EnrichResults(context.Background(), mkResults(5), f, EnrichOptions{Limit: 2})

	if len(f.called()) != 2 {
		t.Errorf("fetched %d urls, want 2", len(f.called()))
	}
	// Only the first two may carry content; the tail must be untouched.
	if res[0].Content == "" || res[1].Content == "" {
		t.Error("leading results should carry content")
	}
	for i := 2; i < len(res); i++ {
		if res[i].Content != "" {
			t.Errorf("result %d beyond limit was fetched", i)
		}
	}
}

func TestEnrichResultsNilFetcherIsNoOp(t *testing.T) {
	res := mkResults(3)
	got := EnrichResults(context.Background(), res, nil, EnrichOptions{})
	for i, r := range got {
		if r.Content != "" {
			t.Errorf("result %d mutated with a nil fetcher", i)
		}
	}
}

func TestEnrichResultsEmptyInput(t *testing.T) {
	f := &fakeFetcher{}
	if got := EnrichResults(context.Background(), nil, f, EnrichOptions{}); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
	if got := len(f.called()); got != 0 {
		t.Errorf("fetcher was called %d times for empty input", got)
	}
}

// TestEnrichResultsFailedFetchKeepsRow is the contract that makes enrichment
// safe to leave on: a dead URL degrades to a normal SERP row rather than
// vanishing and changing the result count.
func TestEnrichResultsFailedFetchKeepsRow(t *testing.T) {
	bad := "https://example.com/b"
	f := &fakeFetcher{fail: map[string]error{bad: errors.New("boom")}}
	res := mkResults(3)

	got := EnrichResults(context.Background(), res, f, EnrichOptions{})

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (failed row must be kept)", len(got))
	}
	found := false
	for i, r := range got {
		if r.URL == bad {
			found = true
			if r.Content != "" {
				t.Errorf("failed result %d has content", i)
			}
			if r.Title == "" || r.URL == "" {
				t.Errorf("failed result %d lost its SERP fields", i)
			}
		}
	}
	if !found {
		t.Fatal("failed result row disappeared")
	}
}

func TestEnrichResultsSkipsEmptyURL(t *testing.T) {
	f := &fakeFetcher{}
	res := []Result{{Title: "no url", URL: "   "}, {Title: "ok", URL: "https://example.com/x"}}

	got := EnrichResults(context.Background(), res, f, EnrichOptions{})

	if len(f.called()) != 1 {
		t.Errorf("fetched %v, want only the non-empty url", f.called())
	}
	if got[0].Content != "" {
		t.Error("blank url should not have been fetched")
	}
	if got[1].Content == "" {
		t.Error("valid url should have content")
	}
}

// TestEnrichResultsConcurrencyBound guards the shared browser allocator: a
// search must not open more tabs at once than an explicit batch scrape does.
func TestEnrichResultsConcurrencyBound(t *testing.T) {
	f := &fakeFetcher{delay: 10 * time.Millisecond}
	res := EnrichResults(context.Background(), mkResults(20), f, EnrichOptions{Concurrency: 3})

	if peak := atomic.LoadInt32(&f.peak); peak > 3 {
		t.Errorf("peak concurrency = %d, want <= 3", peak)
	}
	if peak := atomic.LoadInt32(&f.peak); peak < 2 {
		t.Errorf("peak concurrency = %d, expected real parallelism", peak)
	}
	for i, r := range res {
		if r.Content == "" {
			t.Errorf("result %d missing content under the concurrency bound", i)
		}
	}
}

func TestEnrichResultsRespectsCancellation(t *testing.T) {
	f := &fakeFetcher{delay: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := EnrichResults(ctx, mkResults(10), f, EnrichOptions{Concurrency: 2})

	if got := len(f.called()); got > 2 {
		t.Errorf("made %d calls after cancellation, want the fan-out to stop early", got)
	}
	// The rows themselves must survive regardless.
	if len(res) != 10 {
		t.Errorf("len = %d, want 10", len(res))
	}
}

func TestEnrichResultsDoesNotReorder(t *testing.T) {
	f := &fakeFetcher{}
	in := mkResults(6)
	before := make([]string, len(in))
	for i, r := range in {
		before[i] = r.URL
	}

	got := EnrichResults(context.Background(), in, f, EnrichOptions{})

	for i := range got {
		if got[i].URL != before[i] {
			t.Errorf("position %d changed: %q -> %q", i, before[i], got[i].URL)
		}
	}
}

// TestSearchResultContentOmittedWhenEmpty guards the wire shape: the field is
// additive, so a plain search response must be byte-identical to before.
func TestSearchResultContentOmittedWhenEmpty(t *testing.T) {
	r := Result{URL: "https://example.com"}
	if strings.Contains(mustJSON(t, r), "content") {
		t.Error("empty content must be omitted from the response")
	}
	r.Content = "# hi"
	if !strings.Contains(mustJSON(t, r), `"content":"# hi"`) {
		t.Error("populated content must appear in the response")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
