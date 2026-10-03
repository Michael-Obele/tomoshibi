package search

import (
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/search/engines"
)

func TestRRFScore(t *testing.T) {
	tests := []struct {
		name string
		rank int
		want float64
	}{
		{"rank 1", 1, 1.0 / 61},
		{"rank 2", 2, 1.0 / 62},
		{"rank 10", 10, 1.0 / 70},
		// A malformed rank must not divide by ~60 and dominate the total.
		{"zero clamps to 1", 0, 1.0 / 61},
		{"negative clamps to 1", -5, 1.0 / 61},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rrfScore(tc.rank); got != tc.want {
				t.Errorf("rrfScore(%d) = %v, want %v", tc.rank, got, tc.want)
			}
		})
	}

	t.Run("decays monotonically", func(t *testing.T) {
		for r := 1; r < 50; r++ {
			if rrfScore(r) <= rrfScore(r+1) {
				t.Fatalf("score not decreasing at rank %d", r)
			}
		}
	})
}

func TestWeightFor(t *testing.T) {
	t.Run("single engine gets full weight", func(t *testing.T) {
		if got := weightFor(0, 1); got != 1.0 {
			t.Errorf("weightFor(0,1) = %v, want 1", got)
		}
	})
	t.Run("top engine outranks bottom", func(t *testing.T) {
		top := weightFor(0, 14)
		bottom := weightFor(13, 14)
		if top <= bottom {
			t.Errorf("top %v should exceed bottom %v", top, bottom)
		}
		if top != 1.0 {
			t.Errorf("top weight = %v, want 1.0", top)
		}
		if bottom < 0.5 {
			t.Errorf("bottom weight %v collapses too far; agreement must still count", bottom)
		}
	})
}

// TestFuseAgreementWins is the central claim: cross-engine corroboration must
// outrank a single engine's confident top hit.
func TestFuseAgreementWins(t *testing.T) {
	// "consensus" is ranked by four engines. "solo" is ranked #1 by one.
	fe := []fuseEngine{
		{name: "e1", weight: 1, results: []engines.Result{
			{Title: "solo", URL: "https://solo.example/"},
			{Title: "consensus", URL: "https://consensus.example/"},
		}},
		{name: "e2", weight: 1, results: []engines.Result{
			{Title: "consensus", URL: "https://consensus.example/"},
		}},
		{name: "e3", weight: 1, results: []engines.Result{
			{Title: "consensus", URL: "https://consensus.example/"},
		}},
		{name: "e4", weight: 1, results: []engines.Result{
			{Title: "consensus", URL: "https://consensus.example/"},
		}},
	}

	got := fuse(fe)
	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(got), got)
	}
	if got[0].Title != "consensus" {
		t.Errorf("corroborated result should rank first, got %q (solo=%q)",
			got[0].Title, got[1].Title)
	}
	if got[0].Relevance != 1.0 {
		t.Errorf("top relevance should be 1.0, got %v", got[0].Relevance)
	}
	if got[0].Relevance <= got[1].Relevance {
		t.Errorf("scores must descend: %v vs %v", got[0].Relevance, got[1].Relevance)
	}
}

func TestFuseDedupesCanonicalURLs(t *testing.T) {
	fe := []fuseEngine{
		{name: "e1", results: []engines.Result{
			{Title: "a", URL: "https://example.com/page"},
			{Title: "b", URL: "https://example.com/page?utm_source=n"},
		}},
		{name: "e2", results: []engines.Result{
			{Title: "c", URL: "https://EXAMPLE.com/page#frag"},
		}},
	}
	got := fuse(fe)
	if len(got) != 1 {
		t.Fatalf("tracking params, host case and fragments must dedupe; got %d: %+v", len(got), got)
	}
}

func TestFuseAttributesToBestRanker(t *testing.T) {
	// The document surfaced by both engines should carry the copy from
	// whichever ranked it higher — richer title, not merge order.
	fe := []fuseEngine{
		{name: "low", results: []engines.Result{
			{Title: "generic", URL: "https://x.example/"},
			{Title: "specific from low", URL: "https://y.example/"},
		}},
		{name: "high", results: []engines.Result{
			{Title: "specific from high", URL: "https://y.example/"},
		}},
	}
	got := fuse(fe)
	for _, r := range got {
		if r.URL == "https://y.example/" {
			if r.Title != "specific from high" {
				t.Errorf("want the title from the engine that ranked it first, got %q", r.Title)
			}
			if r.Engine != "high" {
				t.Errorf("want engine attribution to %q, got %q", "high", r.Engine)
			}
		}
	}
}

func TestFuseEmptyAndEdgeCases(t *testing.T) {
	t.Run("no engines", func(t *testing.T) {
		if got := fuse(nil); got != nil {
			t.Errorf("want nil, got %+v", got)
		}
	})
	t.Run("engines with no results", func(t *testing.T) {
		if got := fuse([]fuseEngine{{name: "a"}, {name: "b"}}); got != nil {
			t.Errorf("want nil, got %+v", got)
		}
	})
	t.Run("blank urls are skipped", func(t *testing.T) {
		got := fuse([]fuseEngine{{name: "a", results: []engines.Result{
			{Title: "x", URL: ""}, {Title: "y", URL: "   "},
		}}})
		if len(got) != 0 {
			t.Errorf("blank URLs must not become results: %+v", got)
		}
	})
	t.Run("zero weight does not divide by zero", func(t *testing.T) {
		got := fuse([]fuseEngine{{name: "a", weight: 0, results: []engines.Result{
			{Title: "t", URL: "https://x.example/"},
		}}})
		if len(got) != 1 {
			t.Fatalf("want 1 result, got %+v", got)
		}
	})
}

func TestFuseDeterministic(t *testing.T) {
	build := func() []fuseEngine {
		return []fuseEngine{
			{name: "e1", weight: 1, results: []engines.Result{
				{Title: "a", URL: "https://a.example/"},
				{Title: "b", URL: "https://b.example/"},
			}},
			{name: "e2", weight: 1, results: []engines.Result{
				{Title: "b", URL: "https://b.example/"},
				{Title: "c", URL: "https://c.example/"},
			}},
			{name: "e3", weight: 1, results: []engines.Result{
				{Title: "c", URL: "https://c.example/"},
				{Title: "a", URL: "https://a.example/"},
			}},
		}
	}
	first := fuse(build())
	for i := 0; i < 20; i++ {
		next := fuse(build())
		for j := range first {
			if next[j].URL != first[j].URL {
				t.Fatalf("run %d position %d: %s != %s (ordering must be stable)", i, j, next[j].URL, first[j].URL)
			}
		}
	}
}

func TestFusePreservesPublishedAt(t *testing.T) {
	// MaxAge filtering depends on published dates surviving fusion.
	fe := []fuseEngine{
		{name: "rss", results: []engines.Result{{Title: "n", URL: "https://n.example/"}}},
	}
	got := fuse(fe)
	if len(got) != 1 {
		t.Fatalf("want 1 result, got %+v", got)
	}
	if got[0].PublishedAt != nil {
		t.Error("undated result should stay undated")
	}
}

func TestFuseSingleEnginePreservesOrder(t *testing.T) {
	// With one engine there is nothing to fuse; its own ranking must survive
	// so single-engine deployments are unaffected by this change.
	fe := []fuseEngine{
		{name: "only", weight: 1, results: []engines.Result{
			{Title: "1", URL: "https://e.example/1"},
			{Title: "2", URL: "https://e.example/2"},
			{Title: "3", URL: "https://e.example/3"},
		}},
	}
	got := fuse(fe)
	for i, want := range []string{"1", "2", "3"} {
		if got[i].Title != want {
			t.Errorf("position %d = %q, want %q", i, got[i].Title, want)
		}
	}
	if got[0].Relevance != 1.0 || got[2].Relevance == 1.0 {
		t.Errorf("relevance should still descend: %v %v %v",
			got[0].Relevance, got[1].Relevance, got[2].Relevance)
	}
}
