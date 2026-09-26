package search

import (
	"context"
	"errors"
	"testing"
)

// concentrated builds n results that all live on one domain (Domain filled).
func concentrated(n int, domain string) []Result {
	out := make([]Result, n)
	for i := range out {
		out[i] = Result{
			Title:  "t",
			URL:    "https://" + domain + "/page",
			Domain: domain,
		}
	}
	return out
}

// diverse builds n results on distinct domains.
func diverse(n int) []Result {
	out := make([]Result, n)
	for i := range out {
		host := string(rune('a'+i)) + ".example"
		out[i] = Result{Title: "t", URL: "https://" + host + "/", Domain: host}
	}
	return out
}

// mixed is `same` results on one domain plus `other` results elsewhere.
func mixed(same, other int, domain string) []Result {
	return append(concentrated(same, domain), diverse(other)...)
}

// urlOnly builds n results on one URL host with Domain left empty, to cover
// the URL-host fallback in resultHost.
func urlOnly(n int, base string) []Result {
	out := make([]Result, n)
	for i := range out {
		out[i] = Result{Title: "t", URL: base + string(rune('a'+i))}
	}
	return out
}

func TestIsWeak(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    bool
	}{
		{"empty", nil, false},
		{"single", concentrated(1, "en.wikipedia.org"), false},
		{"below min results", concentrated(4, "en.wikipedia.org"), false},
		{"all one domain", concentrated(5, "en.wikipedia.org"), true},
		{"exactly 80 percent", mixed(8, 2, "en.wikipedia.org"), true},
		{"under 80 percent", mixed(7, 3, "en.wikipedia.org"), false},
		{"healthy spread", diverse(10), false},
		{"domain empty, url host counted", urlOnly(5, "https://en.wikipedia.org/wiki/"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWeak(tt.results); got != tt.want {
				t.Errorf("isWeak() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHybridGatePrefersStrongerBackend: a concentrated first answer must not
// end the chain when a later backend has a diverse set.
func TestHybridGatePrefersStrongerBackend(t *testing.T) {
	weak := concentrated(6, "en.wikipedia.org")
	strong := diverse(6)
	h := &HybridService{services: []Service{
		&stubService{results: weak, count: 100},
		&stubService{results: strong, count: 6},
	}}

	got, total, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 6 || got[0].Domain == "en.wikipedia.org" {
		t.Fatalf("expected the second backend's diverse results, got %#v", got)
	}
	if total != 6 {
		t.Errorf("total = %d, want 6 (the strong backend's)", total)
	}
}

// TestHybridGateKeepsWeakAsLastResort: when nothing stronger answers, the
// concentrated set is still better than an error or empty response.
func TestHybridGateKeepsWeakAsLastResort(t *testing.T) {
	weak := concentrated(6, "en.wikipedia.org")
	h := &HybridService{services: []Service{
		&stubService{results: weak, count: 100},
		&stubService{err: errors.New("backend down")},
	}}

	got, total, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("Search() error = %v, want the weak set returned", err)
	}
	if len(got) != 6 {
		t.Fatalf("len(got) = %d, want 6 (weak set kept)", len(got))
	}
	if total != 100 {
		t.Errorf("total = %d, want 100 (weak set's own total)", total)
	}
}

// TestHybridGateIgnoresSmallSets: below weakMinResults the first backend's
// answer stands, exactly as before the gate existed.
func TestHybridGateIgnoresSmallSets(t *testing.T) {
	small := concentrated(3, "en.wikipedia.org")
	h := &HybridService{services: []Service{
		&stubService{results: small, count: 30},
		&stubService{results: diverse(6), count: 6},
	}}

	got, total, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 3 || total != 30 {
		t.Errorf("expected the first backend's small set untouched, got len=%d total=%d", len(got), total)
	}
}
