package search

import (
	"context"
	"fmt"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/search/engines"
)

func TestRegistrableDomain(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"bare host", "https://example.com/a", "example.com"},
		{"www is not a separate site", "https://www.example.com/a", "example.com"},
		{"language subdomains collapse", "https://en.wikipedia.org/wiki/Go", "wikipedia.org"},
		{"multi-label suffix is NOT split",
			// The case a naive "last two labels" shortcut gets catastrophically
			// wrong: these two must stay distinct or the news category collapses.
			"https://www.bbc.co.uk/news", "bbc.co.uk"},
		{"multi-label suffix, second site", "https://www.theguardian.com/x", "theguardian.com"},
		{"co.uk subdomain", "https://news.bbc.co.uk/x", "bbc.co.uk"},
		{"github.io is itself a public suffix, so each Pages site stands alone",
			// github.io is on the Public Suffix List, so EffectiveTLDPlusOne
			// correctly returns the full host: these are distinct sites and
			// must not be capped as one source.
			"https://rust-lang.github.io/y", "rust-lang.github.io"},
		{"a user site is distinct from the platform", "https://github.com/tokio-rs/tokio", "github.com"},
		{"unrelated host", "https://tokio.rs/", "tokio.rs"},
		{"com.au", "https://www.abc.net.au/news", "abc.net.au"},
		{"ip literal", "http://192.168.1.1:8080/a", "192.168.1.1"},
		{"unparseable is its own bucket, not the empty string",
			// Returning "" here would pool every malformed result together and
			// let the cap silently delete all but N of them.
			"://nope", "://nope"},
		{"empty", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registrableDomain(tt.url); got != tt.want {
				t.Errorf("registrableDomain(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestCapPerDomain(t *testing.T) {
	mk := func(urls ...string) []engines.Result {
		out := make([]engines.Result, len(urls))
		for i, u := range urls {
			out[i] = engines.Result{URL: u, Title: fmt.Sprintf("t%d", i)}
		}
		return out
	}
	urls := func(in []engines.Result) []string {
		out := make([]string, len(in))
		for i, r := range in {
			out[i] = r.URL
		}
		return out
	}

	tests := []struct {
		name string
		in   []engines.Result
		max  int
		want []string
	}{
		{
			name: "uncapped when max is zero",
			in:   mk("https://a.com/1", "https://a.com/2", "https://a.com/3"),
			max:  0,
			want: []string{"https://a.com/1", "https://a.com/2", "https://a.com/3"},
		},
		{
			name: "negative max is uncapped too",
			in:   mk("https://a.com/1", "https://a.com/2"),
			max:  -1,
			want: []string{"https://a.com/1", "https://a.com/2"},
		},
		{
			name: "keeps the first N per domain",
			in:   mk("https://a.com/1", "https://a.com/2", "https://a.com/3", "https://b.com/1"),
			max:  2,
			want: []string{"https://a.com/1", "https://a.com/2", "https://b.com/1"},
		},
		{
			name: "subdomains share the parent's budget",
			in:   mk("https://en.wikipedia.org/a", "https://de.wikipedia.org/b", "https://wikipedia.org/c"),
			max:  2,
			want: []string{"https://en.wikipedia.org/a", "https://de.wikipedia.org/b"},
		},
		{
			name: "www does not buy a second slot",
			in:   mk("https://a.com/1", "https://www.a.com/2"),
			max:  1,
			want: []string{"https://a.com/1"},
		},
		{
			name: "different suffixes stay separate",
			in:   mk("https://bbc.co.uk/1", "https://guardian.co.uk/2"),
			max:  1,
			want: []string{"https://bbc.co.uk/1", "https://guardian.co.uk/2"},
		},
		{
			name: "empty input",
			in:   nil,
			max:  2,
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := urls(capPerDomain(tt.in, tt.max))
			if len(got) != len(tt.want) {
				t.Fatalf("got %d results %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("position %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCapPerDomainIsUncappedForIT is the reason the cap is per-category: a
// code query is answered by GitHub with twenty repos on one host, and capping
// those would throw away the answer rather than diversify it.
func TestCapPerDomainIsUncappedForIT(t *testing.T) {
	reg, err := engines.LoadRegistry("")
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := reg.MaxPerDomainFor("it"); got != 0 {
		t.Errorf("MaxPerDomainFor(it) = %d, want 0 (uncapped)", got)
	}
	if got := reg.MaxPerDomainFor("general"); got != 2 {
		t.Errorf("MaxPerDomainFor(general) = %d, want 2", got)
	}
	if got := reg.MaxPerDomainFor("news"); got != 2 {
		t.Errorf("MaxPerDomainFor(news) = %d, want 2 (falls back to default)", got)
	}
	// An uncategorised query must fall back rather than silently uncapping.
	if got := reg.MaxPerDomainFor("unknown-cat"); got != 2 {
		t.Errorf("MaxPerDomainFor(unknown) = %d, want the default of 2", got)
	}
}

// TestNativeCapKeepsOneEngineFromFillingThePage is the regression this whole
// change exists for: one engine returning ten results from one domain must not
// occupy the whole top 10.
func TestNativeCapKeepsOneEngineFromFillingThePage(t *testing.T) {
	var res []engines.Result
	for i := 0; i < 10; i++ {
		res = append(res, engines.Result{
			Title: fmt.Sprintf("wiki %d", i),
			URL:   fmt.Sprintf("https://en.wikipedia.org/wiki/%d", i),
		})
	}
	res = append(res, engines.Result{Title: "other", URL: "https://pcmag.com/x"})

	wide := NewNativeService([]engines.Engine{&fakeEngine{name: "wiki", cats: []string{"general"}, res: res}})
	narrow := NewNativeService([]engines.Engine{&fakeEngine{name: "wiki", cats: []string{"general"}, res: res}},
		WithMaxPerDomain(map[string]int{"default": 2}))

	opts := SearchOptions{Query: "q", Category: "general", Limit: 10}
	gotWide, _, _, err := wide.SearchWithReport(context.Background(), opts)
	if err != nil {
		t.Fatalf("uncapped search: %v", err)
	}
	gotNarrow, _, _, err := narrow.SearchWithReport(context.Background(), opts)
	if err != nil {
		t.Fatalf("capped search: %v", err)
	}

	wideWiki := countDomain(gotWide, "wikipedia.org")
	narrowWiki := countDomain(gotNarrow, "wikipedia.org")

	if wideWiki <= narrowWiki {
		t.Fatalf("test is not exercising the cap: uncapped=%d capped=%d", wideWiki, narrowWiki)
	}
	if narrowWiki > 2 {
		t.Errorf("capped result set still holds %d wikipedia rows, want <= 2", narrowWiki)
	}
	if len(gotNarrow) < 2 {
		t.Errorf("capping starved the result set: got %d", len(gotNarrow))
	}
}

func TestNativeCapIsPerCategory(t *testing.T) {
	var repo []engines.Result
	for i := 0; i < 6; i++ {
		repo = append(repo, engines.Result{
			Title: fmt.Sprintf("repo %d", i),
			URL:   fmt.Sprintf("https://github.com/org%d/repo", i),
		})
	}
	e := &fakeEngine{name: "gh", cats: []string{"it"}, res: repo}
	n := NewNativeService([]engines.Engine{e}, WithMaxPerDomain(map[string]int{"default": 2, "it": 0}))

	got, _, _, err := n.SearchWithReport(context.Background(), SearchOptions{
		Query: "q", Category: "code", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// "code" maps onto the "it" roster group, which is explicitly uncapped.
	if c := countDomain(got, "github.com"); c != 6 {
		t.Errorf("github.com rows = %d, want 6 (the it category must stay uncapped)", c)
	}
}

func countDomain(rs []Result, want string) int {
	n := 0
	for _, r := range rs {
		if registrableDomain(r.URL) == want {
			n++
		}
	}
	return n
}
