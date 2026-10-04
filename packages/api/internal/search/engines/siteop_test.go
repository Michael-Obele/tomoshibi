package engines

import (
	"net/url"
	"strings"
	"testing"
)

func TestSiteOperatorQuery(t *testing.T) {
	tests := []struct {
		name string
		base string
		q    Query
		want string
	}{
		{
			name: "no filters leaves the query untouched",
			base: "golang concurrency",
			q:    Query{},
			want: "golang concurrency",
		},
		{
			name: "blank filters are not injected",
			base: "golang",
			q:    Query{IncludeDomains: []string{"", "   "}},
			want: "golang",
		},
		{
			name: "empty query is never decorated",
			base: "",
			q:    Query{IncludeDomains: []string{"go.dev"}},
			want: "",
		},
		{
			name: "whitespace-only query is never decorated",
			// Marginalia reads a bare `site:x` as a site-info request, so an
			// empty query must stay empty rather than become one.
			base: "   ",
			q:    Query{IncludeDomains: []string{"go.dev"}},
			want: "",
		},
		{
			name: "single include",
			base: "chi",
			q:    Query{IncludeDomains: []string{"go.dev"}},
			want: "chi site:go.dev",
		},
		{
			name: "include and exclude combine",
			base: "chi",
			q: Query{
				IncludeDomains: []string{"go.dev"},
				ExcludeDomains: []string{"medium.com"},
			},
			want: "chi site:go.dev -site:medium.com",
		},
		{
			name: "multiple includes",
			base: "router",
			q:    Query{IncludeDomains: []string{"go.dev", "github.com"}},
			want: "router site:go.dev site:github.com",
		},
		{
			name: "duplicates collapse",
			base: "chi",
			q:    Query{IncludeDomains: []string{"go.dev", "go.dev", "GO.DEV"}},
			want: "chi site:go.dev",
		},
		{
			name: "wildcard prefix is stripped so the operator stays portable",
			// Marginalia accepts `site:*.x`, DuckDuckGo and Bing do not.
			base: "chi",
			q:    Query{IncludeDomains: []string{"*.example.net"}},
			want: "chi site:example.net",
		},
		{
			name: "scheme and path are stripped",
			base: "chi",
			q:    Query{IncludeDomains: []string{"https://go.dev/blog?x=1"}},
			want: "chi site:go.dev",
		},
		{
			name: "port is stripped",
			base: "chi",
			q:    Query{IncludeDomains: []string{"localhost:8080"}},
			want: "chi site:localhost",
		},
		{
			name: "over-long include list is capped",
			base: "chi",
			q:    Query{IncludeDomains: manyDomains(maxSiteOperators + 5)},
			want: "chi " + strings.Join(siteOps(maxSiteOperators), " "),
		},
		{
			name: "garbage domain is dropped rather than injected",
			base: "chi",
			q:    Query{IncludeDomains: []string{"://"}},
			want: "chi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := siteOperatorQuery(tt.base, tt.q); got != tt.want {
				t.Errorf("siteOperatorQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSiteOperatorEncodeRoundTrip is the property that actually matters at
// the wire: renderURL URL-encodes {{query}}, so the injected operators must
// survive escaping and be readable again by the upstream engine.
func TestSiteOperatorEncodeRoundTrip(t *testing.T) {
	q := Query{
		Q:              "chi router",
		IncludeDomains: []string{"go.dev"},
		ExcludeDomains: []string{"medium.com"},
	}
	q.Q = siteOperatorQuery(q.Q, q)

	raw := renderURL("https://example.com/s?q={{query}}", q)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse rendered url: %v", err)
	}
	got := u.Query().Get("q")
	want := "chi router site:go.dev -site:medium.com"
	if got != want {
		t.Errorf("decoded q = %q, want %q", got, want)
	}
}

// TestSiteOperatorSkippedWhenUndeclared pins the safety property: an engine
// without the capability must receive the caller's query verbatim.
func TestSiteOperatorSkippedWhenUndeclared(t *testing.T) {
	reg, err := LoadRegistry("")
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if reg.Spec("github").SiteOperator {
		t.Error("github must not declare site_operator: a JSON API would match `site:x` literally")
	}
	if reg.Spec("npm").SiteOperator {
		t.Error("npm must not declare site_operator")
	}
	if reg.Spec("wikipedia").SiteOperator {
		t.Error("wikipedia must not declare site_operator")
	}
	for _, name := range []string{"ddg", "brave", "marginalia"} {
		if !reg.Spec(name).SiteOperator {
			t.Errorf("%s should declare site_operator: true", name)
		}
	}
	// Measured 2026-10-04: bing returned 10 results and 0 of them in-domain,
	// i.e. it ignored the operator. An engine that ignores `site:` ranks the
	// injected token as a search term, so the flag must stay off.
	for _, name := range []string{"bing", "bingnews"} {
		if reg.Spec(name).SiteOperator {
			t.Errorf("%s ignored site: in live testing; keep site_operator off", name)
		}
	}
	if reg.Spec("mwmbl").SiteOperator {
		t.Error("mwmbl has no verified site: support; leave it off")
	}
}

func manyDomains(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "d" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".com"
	}
	return out
}

func siteOps(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "site:" + manyDomains(n)[i]
	}
	return out
}
