package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSearXNGServiceParsesResults verifies the SearXNG JSON API is parsed
// into search.Result correctly, including relevance from the engine score.
func TestSearXNGServiceParsesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("expected format=json, got %q", r.URL.Query().Get("format"))
		}
		if r.URL.Query().Get("q") != "golang" {
			t.Errorf("expected q=golang, got %q", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searxngResponse{
			Query:           "golang",
			NumberOfResults: 42,
			Results: []searxngResult{
				{URL: "https://go.dev/", Title: "The Go Programming Language", Content: "Build simple, secure, scalable systems.", Engine: "google", Score: floatPtr(0.9)},
				{URL: "https://example.org/", Title: "Example", Content: "An example page.", Engine: "bing", Score: floatPtr(0.4)},
			},
		})
	}))
	defer srv.Close()

	svc := NewSearXNGService(srv.URL)
	results, total, err := svc.Search(context.Background(), SearchOptions{Query: "golang", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if total != 42 {
		t.Errorf("total = %d, want 42", total)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].URL != "https://go.dev/" || results[0].Title != "The Go Programming Language" {
		t.Errorf("unexpected first result: %+v", results[0])
	}
	if results[0].Relevance != 0.9 {
		t.Errorf("relevance = %v, want 0.9", results[0].Relevance)
	}
	if results[0].Domain != "go.dev" {
		t.Errorf("domain = %q, want go.dev", results[0].Domain)
	}
}

// TestSearXNGServiceHonorsLimit verifies the limit caps returned results.
func TestSearXNGServiceHonorsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searxngResponse{
			Results: []searxngResult{
				{URL: "https://a.dev/", Title: "A"},
				{URL: "https://b.dev/", Title: "B"},
				{URL: "https://c.dev/", Title: "C"},
			},
		})
	}))
	defer srv.Close()

	svc := NewSearXNGService(srv.URL)
	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 2})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

// TestHybridChainFallsThrough verifies the fallback chain tries the next
// backend when the first fails.
func TestHybridChainFallsThrough(t *testing.T) {
	calls := 0
	first := &stubService{err: context.DeadlineExceeded}
	second := &stubService{results: []Result{{Title: "ok", URL: "https://ok.dev/"}}, count: 1}

	h := &HybridService{services: []Service{first, second}}
	results, _, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	calls = first.calls + second.calls
	if calls != 2 {
		t.Errorf("expected both backends called, got %d calls", calls)
	}
	if len(results) != 1 || results[0].Title != "ok" {
		t.Errorf("expected fallback results, got %+v", results)
	}
}

type stubService struct {
	results []Result
	count   int
	err     error
	calls   int
}

func (s *stubService) Search(_ context.Context, _ SearchOptions) ([]Result, int, error) {
	s.calls++
	return s.results, s.count, s.err
}

func floatPtr(f float64) *float64 { return &f }
