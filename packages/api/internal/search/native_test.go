package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/search/engines"
)

// fakeEngine is a scripted engines.Engine for fan-out tests.
type fakeEngine struct {
	name string
	cats []string
	res  []engines.Result
	err  error
}

func (f *fakeEngine) Name() string         { return f.name }
func (f *fakeEngine) Categories() []string { return f.cats }
func (f *fakeEngine) Search(context.Context, engines.Query) ([]engines.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.res, nil
}

func res(title, url string) engines.Result {
	return engines.Result{Title: title, URL: url, Description: title + " desc", Engine: ""}
}

func TestNativeMergeOrderAndDedup(t *testing.T) {
	a := &fakeEngine{name: "first", cats: []string{"general"}, res: []engines.Result{
		res("A1", "https://example.com/a1"),
		res("A2", "https://example.com/a2?utm_source=x"),
	}}
	b := &fakeEngine{name: "second", cats: []string{"general"}, res: []engines.Result{
		res("B1", "https://example.com/a2"),        // dup of A2 (no utm)
		res("B2", "https://other.org/b2#fragment"), // fragment dup target
		res("B2f", "https://other.org/b2"),         // same after fragment strip
	}}
	n := NewNativeService([]engines.Engine{a, b})

	got, total, err := n.Search(context.Background(), SearchOptions{Query: "q", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// a2 (utm variant) and a2 (clean) share one canonical key; #fragment
	// and bare form likewise — first occurrence wins, so 5 raw → 3 unique.
	if total != 3 {
		t.Errorf("total = %d, want 3 (deduped)", total)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 results after dedup, got %d: %+v", len(got), urls(got))
	}
	if got[0].Engine != "first" || got[2].Engine != "second" {
		t.Errorf("engine attribution wrong: %s … %s", got[0].Engine, got[2].Engine)
	}
	if got[0].Relevance < got[2].Relevance {
		t.Errorf("earlier engine should outrank later: %.2f < %.2f", got[0].Relevance, got[2].Relevance)
	}
	if got[0].Domain != "example.com" || got[0].ID == "" {
		t.Errorf("metadata not populated: %+v", got[0])
	}
}

func urls(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.URL
	}
	return out
}

func TestNativeCategoryMapping(t *testing.T) {
	it := &fakeEngine{name: "coder", cats: []string{"it"}, res: []engines.Result{res("R", "https://x.dev/1")}}
	news := &fakeEngine{name: "newsy", cats: []string{"news"}, res: []engines.Result{res("N", "https://x.dev/n")}}
	n := NewNativeService([]engines.Engine{it, news})

	// "code" (API vocabulary) must reach the "it" registry group.
	if _, _, err := n.Search(context.Background(), SearchOptions{Query: "q", Category: "code"}); err != nil {
		t.Fatalf("code → it mapping failed: %v", err)
	}
	if _, _, err := n.Search(context.Background(), SearchOptions{Query: "q", Category: "news"}); err != nil {
		t.Fatalf("news: %v", err)
	}
	// General has no engines registered → explicit error, not silent empty.
	if _, _, err := n.Search(context.Background(), SearchOptions{Query: "q", Category: "general"}); err == nil {
		t.Fatal("expected error for category with no engines")
	}
	if _, _, err := n.Search(context.Background(), SearchOptions{Query: "q", Category: "music"}); err == nil {
		t.Fatal("invalid category must fail validation")
	}
}

func TestNativeAllFailReturnsError(t *testing.T) {
	boom := errors.New("upstream exploded")
	n := NewNativeService([]engines.Engine{
		&fakeEngine{name: "a", cats: []string{"general"}, err: boom},
		&fakeEngine{name: "b", cats: []string{"general"}, err: fmt.Errorf("%w: HTTP 429", engines.ErrBlocked)},
	})
	_, _, report, err := n.SearchWithReport(context.Background(), SearchOptions{Query: "q"})
	if err == nil {
		t.Fatal("all-engine failure must surface an error (hybrid falls through)")
	}
	if len(report.Outcomes) != 2 {
		t.Errorf("report should still carry both outcomes: %+v", report.Outcomes)
	}
	if un := report.Unresponsive(); len(un) != 2 {
		t.Errorf("unresponsive tuples = %d, want 2", len(un))
	}
}

func TestNativePartialFailureIsNotError(t *testing.T) {
	bad := &fakeEngine{name: "bad", cats: []string{"general"},
		err: fmt.Errorf("%w: HTTP 403", engines.ErrBlocked)}
	good := &fakeEngine{name: "good", cats: []string{"general"},
		res: []engines.Result{res("OK", "https://ok.dev/1")}}
	n := NewNativeService([]engines.Engine{bad, good})

	got, _, report, err := n.SearchWithReport(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("one healthy engine must keep search alive: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("results = %d, want 1", len(got))
	}
	un := report.Unresponsive()
	if len(un) != 1 || un[0][0] != "bad" {
		t.Fatalf("unresponsive = %+v, want [[bad …]]", un)
	}
	if !strings.Contains(un[0][1], "blocked") {
		t.Errorf("reason should mention block: %q", un[0][1])
	}
}

func TestNativeNotConfiguredReason(t *testing.T) {
	skipped := &fakeEngine{name: "keyed", cats: []string{"general"},
		err: fmt.Errorf("%w: set SOME_KEY", engines.ErrNotConfigured)}
	good := &fakeEngine{name: "good", cats: []string{"general"},
		res: []engines.Result{res("OK", "https://ok.dev/1")}}
	n := NewNativeService([]engines.Engine{skipped, good})
	_, _, report, err := n.SearchWithReport(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	un := report.Unresponsive()
	if len(un) != 1 || un[0][1] != "not configured" {
		t.Fatalf("unresponsive = %+v", un)
	}
}

func TestNativeFilters(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	fresh := time.Now().Add(-1 * time.Hour)
	e := &fakeEngine{name: "e", cats: []string{"general"}, res: []engines.Result{
		{Title: "keep", URL: "https://blog.example.com/keep", Engine: "e"},
		{Title: "exclude", URL: "https://spam.example.org/x", Engine: "e"},
		{Title: "stale", URL: "https://news.example.net/stale", Engine: "e", Published: &old},
		{Title: "fresh", URL: "https://news.example.net/fresh", Engine: "e", Published: &fresh},
	}}
	n := NewNativeService([]engines.Engine{e})

	maxAge := 1
	got, _, err := n.Search(context.Background(), SearchOptions{
		Query: "q", Category: "general", Limit: 10,
		IncludeDomains: []string{"example.com", "*.example.net"},
		ExcludeDomains: []string{"spam.example.org"},
		MaxAge:         &maxAge,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	want := []string{"https://blog.example.com/keep", "https://news.example.net/fresh"}
	if fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("filtered = %v, want %v", urls(got), want)
	}
}

func TestNativeLimitAndEmptyQuery(t *testing.T) {
	res25 := make([]engines.Result, 0, 25)
	for i := 0; i < 25; i++ {
		res25 = append(res25, res(fmt.Sprintf("R%d", i), fmt.Sprintf("https://x.dev/%d", i)))
	}
	n := NewNativeService([]engines.Engine{
		&fakeEngine{name: "e", cats: []string{"general"}, res: res25},
	})
	got, _, err := n.Search(context.Background(), SearchOptions{Query: "q", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("limit not honored: %d results", len(got))
	}
	if _, _, err := n.Search(context.Background(), SearchOptions{Query: "   "}); err == nil {
		t.Error("empty query must be rejected")
	}
}
