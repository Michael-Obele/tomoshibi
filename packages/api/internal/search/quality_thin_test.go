package search

import (
	"context"
	"fmt"
	"testing"
)

func wikiSet(n int) []Result {
	out := make([]Result, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Result{
			URL:    fmt.Sprintf("https://en.wikipedia.org/page%d", i),
			Domain: "en.wikipedia.org",
		})
	}
	return out
}

// TestIsThinBoundaries pins the exact cut-off so a future tweak to
// thinMinResults is a deliberate, visible change.
func TestIsThinBoundaries(t *testing.T) {
	t.Run("four same-host rows cross the threshold", func(t *testing.T) {
		// At thinMinResults the set is no longer "thin" — it has enough rows
		// to stand on its own, and the concentration check takes over at 5.
		if isThin(wikiSet(4)) {
			t.Error("a 4-row set should not be thin")
		}
	})
	t.Run("three same-host rows are thin", func(t *testing.T) {
		if !isThin(wikiSet(3)) {
			t.Error("a 3-row same-host set should be thin")
		}
	})
	t.Run("three rows across domains are not thin", func(t *testing.T) {
		if isThin([]Result{
			{URL: "https://a.example/", Domain: "a.example"},
			{URL: "https://b.example/", Domain: "b.example"},
			{URL: "https://c.example/", Domain: "c.example"},
		}) {
			t.Error("a diverse 3-row set is a legitimate narrow answer, not a blocked upstream")
		}
	})
}

// TestThinSetFallsThroughChain is the behavioural payoff: a thin, single-host
// answer must not end the fallback chain, and must still be returned when
// nothing stronger exists.
func TestThinSetFallsThroughChain(t *testing.T) {
	thin := &stubService{results: wikiSet(2), count: 1}
	var strongResults []Result
	for i := 0; i < 8; i++ {
		strongResults = append(strongResults, Result{
			URL:    fmt.Sprintf("https://site%d.example/p", i),
			Domain: fmt.Sprintf("site%d.example", i),
		})
	}
	strong := &stubService{results: strongResults, count: 1}

	h := &HybridService{services: []Service{thin, strong}}

	got, _, err := h.Search(context.Background(), SearchOptions{Query: "q", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		// 8 strong + 2 thin: the thin set is kept as a tail, not discarded,
		// matching the existing weak-set merge behaviour.
		t.Errorf("expected strong set plus merged thin tail (8+2=10), got %d", len(got))
	}
	if got[0].Domain == "en.wikipedia.org" {
		t.Error("the strong set must lead; the thin set belongs in the tail")
	}

	t.Run("thin set is still returned when nothing better exists", func(t *testing.T) {
		h2 := &HybridService{services: []Service{thin}}
		got, _, err := h2.Search(context.Background(), SearchOptions{Query: "q", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("thin results must not be discarded entirely, got %d", len(got))
		}
	})
}
