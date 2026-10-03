package search

import (
	"fmt"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/search/engines"
)

// legacyResult mirrors what the retired relevanceAt scheme produced.
type legacyResult struct {
	u      string
	engine string
	rel    float64
}

// legacyRank reproduces the pre-RRF merge so the two schemes can be compared
// side by side on identical input. This is test-only scaffolding that
// documents the behaviour RRF replaced; it is not used in production.
func legacyRank(raw [][]engineRes, names []string) []legacyResult {
	var out []legacyResult
	seen := map[string]bool{}
	base := 0
	for i, list := range raw {
		for _, r := range list {
			key := canonicalURL(r.u)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, legacyResult{
				u:      r.u,
				engine: names[i],
				rel:    relevanceAtLegacy(len(raw), i, base),
			})
			base++
		}
	}
	return out
}

type engineRes struct {
	u string
	t string
}

// relevanceAtLegacy is the retired scoring function, preserved here only so
// the tests can demonstrate what changed.
func relevanceAtLegacy(numEngines, engineIdx, position int) float64 {
	base := 1.0 - float64(engineIdx)*0.08
	if base < 0.2 {
		base = 0.2
	}
	rel := base - float64(position)*0.015
	if rel < 0.05 {
		rel = 0.05
	}
	return rel
}

func toEngines(in []engineRes) []engines.Result {
	out := make([]engines.Result, 0, len(in))
	for _, r := range in {
		out = append(out, engines.Result{Title: r.t, URL: r.u})
	}
	return out
}

// TestSchemeComparison prints the old and new rankings for one realistic
// query so the difference is visible, and asserts the properties that make
// RRF an improvement.
func TestSchemeComparison(t *testing.T) {
	raw := [][]engineRes{
		// ddg — the strongest-weight engine here. Its #1 is a page only it
		// found; nothing else in the roster agrees it is the best answer.
		{
			{u: "https://go.dev/wiki", t: "Go Wiki (ddg only)"},
			{u: "https://go.dev/blog", t: "The Go Blog (ddg #2)"},
			{u: "https://pkg.go.dev/net/http", t: "net/http (ddg #3)"},
		},
		// wikipedia
		{
			{u: "https://en.wikipedia.org/wiki/Go_(programming_language)", t: "Go (programming language)"},
			{u: "https://go.dev/blog", t: "The Go Blog (wikipedia #2)"},
		},
		// github
		{
			{u: "https://github.com/golang/go", t: "golang/go"},
			{u: "https://go.dev/blog", t: "The Go Blog (github #2)"},
			{u: "https://pkg.go.dev/net/http", t: "net/http (github #3)"},
		},
	}
	names := []string{"ddg", "wikipedia", "github"}

	fe := make([]fuseEngine, 0, len(raw))
	for i, list := range raw {
		fe = append(fe, fuseEngine{name: names[i], weight: weightFor(i, len(raw)), results: toEngines(list)})
	}

	old := legacyRank(raw, names)
	got := fuse(fe)

	t.Log("OLD (synthetic decay: engine weight - position*0.015):")
	for i, r := range old {
		t.Logf("  %d. %-46s rel=%.3f engine=%s", i+1, r.u, r.rel, r.engine)
	}
	t.Log("NEW (reciprocal rank fusion):")
	for i, r := range got {
		t.Logf("  %d. %-46s rel=%.3f engine=%s", i+1, r.URL, r.Relevance, r.Engine)
	}

	fmt.Println()
	fmt.Println("=== OLD (engine weight - position*0.015) ===")
	for i, r := range old {
		fmt.Printf("  %d. %-46s rel=%.3f  (%s)\n", i+1, r.u, r.rel, r.engine)
	}
	fmt.Println("\n=== NEW (reciprocal rank fusion) ===")
	for i, r := range got {
		fmt.Printf("  %d. %-46s rel=%.3f  (%s)\n", i+1, r.URL, r.Relevance, r.Engine)
	}

	// go.dev/blog is surfaced by all three engines (each at #2), while
	// go.dev/wiki is ddg's lone #1. Consensus must win.
	if got[0].URL != "https://go.dev/blog" {
		t.Errorf("three-engine consensus should rank first, got %s", got[0].URL)
	}
	// And the old scheme demonstrably got this wrong.
	if old[0].u == "https://go.dev/blog" {
		t.Error("expected the old scheme to rank ddg's lone #1 above the consensus; " +
			"if this fails the example no longer demonstrates the improvement")
	} else {
		t.Logf("confirmed: old scheme put %q first, RRF promotes the consensus instead", old[0].u)
	}
}
