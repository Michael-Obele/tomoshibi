package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHybridServiceWithStealthChain(t *testing.T) {
	fetcher := &stubFetcher{html: `<html><body><a href="https://a.dev/">Title A long enough</a></body></html>`}
	svc := NewHybridServiceWithStealth("test-key", "http://searxng:8080", fetcher)
	h, ok := svc.(*HybridService)
	if !ok {
		t.Fatalf("expected *HybridService, got %T", svc)
	}
	if len(h.services) != 3 {
		t.Errorf("expected 3 backends, got %d", len(h.services))
	}
	if _, ok := h.services[0].(*SearXNGService); !ok {
		t.Errorf("first should be SearXNG, got %T", h.services[0])
	}
	if _, ok := h.services[1].(*StealthService); !ok {
		t.Errorf("second should be Stealth (free fallback before metered Brave), got %T", h.services[1])
	}
	if _, ok := h.services[2].(*BraveService); !ok {
		t.Errorf("third should be Brave (metered last resort), got %T", h.services[2])
	}
}

func TestHybridChainFallsThroughToStealth(t *testing.T) {
	first := &stubService{err: fmt.Errorf("searxng 429")}
	second := &stubService{err: fmt.Errorf("brave 429")}
	third := &stubService{results: []Result{{Title: "stealth hit", URL: "https://stealth.dev/"}}, count: 1}
	h := &HybridService{services: []Service{first, second, third}}
	results, _, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("expected stealth fallback, got error %v", err)
	}
	if len(results) != 1 || results[0].Title != "stealth hit" {
		t.Errorf("expected stealth results, got %+v", results)
	}
}

func TestHybridChainFallsThroughOnEmpty(t *testing.T) {
	first := &stubService{results: nil, count: 0}
	second := &stubService{results: []Result{{Title: "brave hit", URL: "https://brave.dev/"}}, count: 1}
	h := &HybridService{services: []Service{first, second}}
	results, _, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if len(results) != 1 || results[0].Title != "brave hit" {
		t.Errorf("expected fallback on empty, got %+v", results)
	}
}

func TestSearXNGSuspendedEngineFallsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searxngResponse{
			Query: "q", NumberOfResults: 0, Results: nil,
		})
	}))
	defer srv.Close()
	searxng := NewSearXNGService(srv.URL)
	stealthFetcher := &stubFetcher{html: `<html><body><a href="https://a.dev/">Title A long enough</a></body></html>`}
	stealth := NewStealthService(stealthFetcher, "https://search.brave.com")
	h := &HybridService{services: []Service{searxng, stealth}}
	results, _, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if len(results) == 0 {
		t.Error("expected stealth fallback when SearXNG returns empty")
	}
}

func TestWiringStealthDisabledByDefault(t *testing.T) {
	svc := NewHybridServiceWithStealth("key", "http://searxng:8080", nil)
	h, ok := svc.(*HybridService)
	if !ok {
		t.Fatalf("expected HybridService, got %T", svc)
	}
	if len(h.services) != 2 {
		t.Errorf("without fetcher, should be 2 backends, got %d", len(h.services))
	}
}
