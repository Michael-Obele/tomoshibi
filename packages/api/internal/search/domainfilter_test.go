package search

import (
	"context"
	"testing"
)

// stubBackend is a fixed-answer Service used to exercise the chain.
type stubBackend struct {
	name string
	res  []Result
	err  error
	// gotOpts records the options this backend was called with.
	gotOpts SearchOptions
}

func (s *stubBackend) Search(_ context.Context, opts SearchOptions) ([]Result, int, error) {
	s.gotOpts = opts
	return s.res, len(s.res), s.err
}

func stub(name string, domains ...string) *stubBackend {
	b := &stubBackend{name: name}
	for i, d := range domains {
		b.res = append(b.res, Result{
			Title:  d,
			URL:    "https://" + d + "/p",
			Domain: d,
			ID:     d + string(rune('0'+i)),
		})
	}
	return b
}

// TestIsWeakRespectsDomainPinnedSearch pins the regression: a caller who asks
// for one domain should get that domain back, not have the set judged
// "concentrated" and passed over for a worse backend.
func TestIsWeakRespectsDomainPinnedSearch(t *testing.T) {
	concentrated := []Result{
		{URL: "https://go.dev/a", Domain: "go.dev"},
		{URL: "https://go.dev/b", Domain: "go.dev"},
		{URL: "https://go.dev/c", Domain: "go.dev"},
		{URL: "https://go.dev/d", Domain: "go.dev"},
		{URL: "https://go.dev/e", Domain: "go.dev"},
		{URL: "https://go.dev/f", Domain: "go.dev"},
	}

	// Unfiltered: the classic blocked-upstream signature, correctly weak.
	if !isWeak(concentrated, SearchOptions{}) {
		t.Error("an unfiltered single-domain set should still count as weak")
	}

	// Pinned to that domain: concentration is the requested shape.
	if isWeak(concentrated, SearchOptions{IncludeDomains: []string{"go.dev"}}) {
		t.Error("a domain-pinned search must not be judged weak for concentration")
	}

	// ExcludeDomains does not pin the shape, so the gate still applies.
	if !isWeak(concentrated, SearchOptions{ExcludeDomains: []string{"medium.com"}}) {
		t.Error("excludeDomains must not disable the concentration gate")
	}
}

// TestHybridAppliesDomainFilterToFallbackBackend is the other regression:
// domain filters used to be applied by NativeService only, so a chain that
// fell through to SearXNG/Stealth/Brave returned unfiltered rows.
//
// The native stub here is deliberately the blocked-upstream signature
// (6 rows, one host) so the chain really does walk past it and reach the
// fallback — otherwise the test would pass even with the filter missing,
// because the fallback would never be consulted.
func TestHybridAppliesDomainFilterToFallbackBackend(t *testing.T) {
	native := stub("native", "en.wikipedia.org", "en.wikipedia.org",
		"en.wikipedia.org", "en.wikipedia.org", "en.wikipedia.org", "en.wikipedia.org")
	fallback := stub("searxng", "medium.com", "dev.to", "go.dev")
	h := &HybridService{services: []Service{native, fallback}}

	opts := SearchOptions{Query: "router", Limit: 10, IncludeDomains: []string{"go.dev"}}
	got, _, err := h.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected the in-domain fallback results to survive")
	}
	for _, r := range got {
		if r.Domain != "go.dev" {
			t.Errorf("leaked off-domain result %q from the fallback backend", r.Domain)
		}
	}
}

// TestHybridSkipsBackendFilteredToEmpty covers a fallback whose entire result
// set is filtered away: it must not be reported as having answered.
func TestHybridSkipsBackendFilteredToEmpty(t *testing.T) {
	native := stub("native", "medium.com")
	clean := stub("searxng", "go.dev", "go.dev", "pkg.go.dev")
	h := &HybridService{services: []Service{native, clean}}

	opts := SearchOptions{Query: "router", Limit: 10, IncludeDomains: []string{"go.dev"}}
	got, _, err := h.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected results from the backend that had in-domain hits")
	}
	for _, r := range got {
		if r.Domain == "medium.com" {
			t.Error("a fully filtered-out backend still contributed rows")
		}
	}
}

// TestHybridAppliesExcludeDomainsToFallback guards the other direction.
func TestHybridAppliesExcludeDomainsToFallback(t *testing.T) {
	native := stub("native", "go.dev")
	fallback := stub("searxng", "medium.com", "go.dev")
	h := &HybridService{services: []Service{native, fallback}}

	opts := SearchOptions{Query: "router", Limit: 10, ExcludeDomains: []string{"medium.com"}}
	got, _, err := h.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range got {
		if r.Domain == "medium.com" {
			t.Error("excludeDomains was ignored on the fallback backend")
		}
	}
}
