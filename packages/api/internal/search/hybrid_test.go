package search

import (
	"context"
	"strings"
	"testing"
)

// TestNewHybridServiceNoBackend verifies that with neither SearXNG nor Brave
// configured, search fails with a clear configuration error instead of
// silently returning empty results.
func TestNewHybridServiceNoBackend(t *testing.T) {
	svc := NewHybridService("", "")
	if _, ok := svc.(noBackendService); !ok {
		t.Fatalf("expected noBackendService, got %T", svc)
	}
	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "q"})
	if err == nil || !strings.Contains(err.Error(), "SEARXNG_ENDPOINT") {
		t.Errorf("expected config error mentioning SEARXNG_ENDPOINT, got %v", err)
	}
}

// TestNewHybridServiceSearXNGOnly verifies a SearXNG-only deployment returns
// the SearXNG service directly (no wrapper indirection).
func TestNewHybridServiceSearXNGOnly(t *testing.T) {
	svc := NewHybridService("", "http://searxng:8080")
	if _, ok := svc.(*SearXNGService); !ok {
		t.Fatalf("expected *SearXNGService, got %T", svc)
	}
}

// TestNewHybridServiceBraveOnly verifies a Brave-only deployment returns the
// Brave service directly.
func TestNewHybridServiceBraveOnly(t *testing.T) {
	svc := NewHybridService("test-key", "")
	if _, ok := svc.(*BraveService); !ok {
		t.Fatalf("expected *BraveService, got %T", svc)
	}
}

// TestNewHybridServiceChain verifies the chain is ordered SearXNG first, then
// Brave, so the free self-hosted backend wins and Brave is the fallback.
func TestNewHybridServiceChain(t *testing.T) {
	svc := NewHybridService("test-key", "http://searxng:8080")
	h, ok := svc.(*HybridService)
	if !ok {
		t.Fatalf("expected *HybridService, got %T", svc)
	}
	if len(h.services) != 2 {
		t.Errorf("expected 2 backends, got %d", len(h.services))
	}
	if _, ok := h.services[0].(*SearXNGService); !ok {
		t.Errorf("first backend should be SearXNG, got %T", h.services[0])
	}
	if _, ok := h.services[1].(*BraveService); !ok {
		t.Errorf("second backend should be Brave, got %T", h.services[1])
	}
}
