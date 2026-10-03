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
			URL:    "https://" + domain + "/page-" + string(rune('a'+i)),
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
		// Small same-host sets now count as weak: a blocked upstream that
		// answers with one or two rows must not end the fallback chain.
		{"single row, one host", concentrated(1, "en.wikipedia.org"), true},
		{"two rows, one host", concentrated(2, "en.wikipedia.org"), true},
		{"three rows, one host", concentrated(3, "en.wikipedia.org"), true},
		// Four rows cross the thinness threshold, so the concentration gate
		// (which starts at 5) applies and the set stands on its own.
		{"four rows, one host", concentrated(4, "en.wikipedia.org"), false},
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
// end the chain when a later backend has a diverse set — and its hits are
// merged behind the strong set instead of discarded.
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
	if len(got) != 12 {
		t.Fatalf("len(got) = %d, want 12 (strong 6 + weak 6 merged)", len(got))
	}
	if got[0].Domain == "en.wikipedia.org" {
		t.Error("the strong set must lead the merged list")
	}
	weakKept := false
	for _, r := range got {
		if r.Domain == "en.wikipedia.org" {
			weakKept = true
		}
	}
	if !weakKept {
		t.Error("the weak backend's hits must survive the merge")
	}
	if total != 12 {
		t.Errorf("total = %d, want 12 (the merged length)", total)
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
	if total != 6 {
		t.Errorf("total = %d, want 6 (the kept weak set's length)", total)
	}
}

// TestHybridGateMergesTwoWeakBackends: with no strong backend at all, two
// concentrated sets are unioned rather than returning only the first.
func TestHybridGateMergesTwoWeakBackends(t *testing.T) {
	first := concentrated(6, "en.wikipedia.org")
	second := concentrated(6, "wikihow.com")
	h := &HybridService{services: []Service{
		&stubService{results: first, count: 6},
		&stubService{results: second, count: 6},
	}}

	got, total, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 12 || total != 12 {
		t.Errorf("len/total = %d/%d, want 12/12 (both weak sets merged)", len(got), total)
	}
}

// TestHybridGateHedgesThinSets: a small, single-host answer no longer ends
// the chain. It was previously taken at face value, which meant a blocked
// upstream answering with a token 3-row response beat every fallback behind
// it. The chain now continues, and the thin hits are merged into the tail of
// the stronger set rather than discarded.
func TestHybridGateHedgesThinSets(t *testing.T) {
	thin := concentrated(3, "en.wikipedia.org")
	strong := diverse(6)
	h := &HybridService{services: []Service{
		&stubService{results: thin, count: 30},
		&stubService{results: strong, count: 6},
	}}

	got, _, err := h.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 9 {
		t.Fatalf("expected strong 6 + thin 3 merged = 9, got %d", len(got))
	}
	if got[0].Domain == "en.wikipedia.org" {
		t.Error("the strong set must lead the merged list")
	}
	kept := false
	for _, r := range got {
		if r.Domain == "en.wikipedia.org" {
			kept = true
		}
	}
	if !kept {
		t.Error("thin results must survive as a tail rather than being dropped")
	}
}
