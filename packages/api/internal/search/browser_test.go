package search

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// serpFetcher serves one page and remembers what it was asked for, so a test can
// assert the URL the engine was driven to (udm=14, count, first) as well as the
// parse.
type serpFetcher struct {
	html string
	err  error
	urls []string
}

func (s *serpFetcher) FetchHTML(_ context.Context, rawURL string) (string, error) {
	s.urls = append(s.urls, rawURL)
	return s.html, s.err
}

const ddgHTML = `<!doctype html><html><body>
<div class="results">
  <div class="result">
    <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fsvelte.dev%2Fdocs&rut=abc">Svelte docs</a>
    <a class="result__snippet">The official documentation.</a>
  </div>
  <div class="result">
    <a class="result__a" href="https://kit.svelte.dev/docs">SvelteKit docs</a>
    <a class="result__snippet">Routing, load and form actions.</a>
  </div>
  <div class="result">
    <a href="https://duckduckgo.com/y.js?ad=1">Sponsored</a>
  </div>
</div>
</body></html>`

const googleHTML = `<!doctype html><html><body>
<div id="search"><div id="rso">
  <div class="MjjYud">
    <a href="/url?q=https://svelte.dev/blog/runes&sa=U"><h3>Introducing runes</h3></a>
    <div class="VwiC3b">Runes are the new reactivity primitive.</div>
  </div>
  <div class="MjjYud">
    <a href="/url?q=https://www.google.com/preferences&sa=U"><h3>Search settings</h3></a>
    <div>Preferences</div>
  </div>
</div></div>
</body></html>`

const bingHTML = `<!doctype html><html><body>
<ol id="b_results">
  <li class="b_algo">
    <h2><a href="https://example.org/article">Example article</a></h2>
    <div class="b_caption"><p>An excerpt from the page.</p></div>
  </li>
</ol>
</body></html>`

// TestBrowserEngineParsesEachEngine is the contract that makes ddg/google/bing
// swappable: one Search call, one Result shape, whatever answered.
func TestBrowserEngineParsesEachEngine(t *testing.T) {
	cases := []struct {
		engine    string
		html      string
		wantURL   string
		wantTitle string
		wantLen   int
	}{
		// The DuckDuckGo link is a //duckduckgo.com/l/?uddg= wrapper; the desired
		// URL is what a caller must receive, not the wrapper.
		{"ddg", ddgHTML, "https://svelte.dev/docs", "Svelte docs", 2},
		// Google's href is a /url?q= wrapper, and its own settings page must be
		// filtered out rather than returned as a result.
		{"google", googleHTML, "https://svelte.dev/blog/runes", "Introducing runes", 1},
		{"bing", bingHTML, "https://example.org/article", "Example article", 1},
	}

	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			f := &serpFetcher{html: tc.html}
			svc := NewBrowserService(f, tc.engine)

			results, total, err := svc.Search(context.Background(), SearchOptions{Query: "svelte runes", Limit: 10})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if total != tc.wantLen || len(results) != tc.wantLen {
				t.Fatalf("got %d results (%d total), want %d: %+v", len(results), total, tc.wantLen, results)
			}
			if results[0].URL != tc.wantURL {
				t.Fatalf("URL = %q, want %q (the wrapper must be unwrapped)", results[0].URL, tc.wantURL)
			}
			if results[0].Title != tc.wantTitle {
				t.Fatalf("Title = %q, want %q", results[0].Title, tc.wantTitle)
			}
			if results[0].Engine != "browser:"+tc.engine {
				t.Fatalf("Engine = %q, want browser:%s", results[0].Engine, tc.engine)
			}
			if results[0].Domain == "" {
				t.Fatalf("Domain should be filled in: %+v", results[0])
			}
			for _, r := range results {
				if strings.Contains(r.URL, "duckduckgo.com") || strings.Contains(r.URL, "google.com") {
					t.Fatalf("an engine's own URL leaked into results: %q", r.URL)
				}
			}
		})
	}
}

// TestBrowserEngineURLShapes pins the query parameters that decide whether a
// SERP is parseable at all. `udm=14` is not decoration: it is the Web filter,
// which removes the AI overview that otherwise pushes real results off the page.
func TestBrowserEngineURLShapes(t *testing.T) {
	cases := []struct {
		engine string
		limit  int
		offset int
		want   []string
	}{
		{"google", 20, 10, []string{"udm=14", "num=20", "start=10"}},
		{"bing", 15, 10, []string{"count=15", "first=11"}},
		{"ddg", 10, 20, []string{"s=20", "html.duckduckgo.com"}},
	}
	for _, tc := range cases {
		got := serpURL(tc.engine, "a b", tc.limit, tc.offset)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("serpURL(%s) = %q, missing %q", tc.engine, got, want)
			}
		}
	}
}

// TestBrowserEngineRefusesAChallenge: an interstitial arrives as HTTP 200 with
// no results. Returning an empty list would be a lie the caller cannot see, and
// retrying is what turns a warning into a suspension.
func TestBrowserEngineRefusesAChallenge(t *testing.T) {
	for _, body := range []string{
		"<html><body>Our systems have detected unusual traffic from your computer network.</body></html>",
		"<html><body>Please enable JavaScript and cookies to continue</body></html>",
		"<html><body><div class='anomaly-modal'>bots use DuckDuckGo too</div></body></html>",
	} {
		svc := NewBrowserService(&serpFetcher{html: body}, "google")
		_, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 5})
		if !errors.Is(err, ErrBrowserChallenge) {
			t.Fatalf("want ErrBrowserChallenge for %q, got %v", body, err)
		}
	}
}

// TestBrowserEngineSelection: one instance can serve any engine, which is what
// makes "try google this time" a parameter instead of a restart.
func TestBrowserEngineSelection(t *testing.T) {
	f := &serpFetcher{html: bingHTML}
	svc := NewBrowserService(f, BrowserDDG)
	if svc.Engine() != BrowserDDG {
		t.Fatalf("default engine should be ddg, got %q", svc.Engine())
	}
	if _, _, err := svc.Search(context.Background(), SearchOptions{
		Query: "x", Limit: 5, BrowserEngine: BrowserBing,
	}); err != nil {
		t.Fatalf("Search with engine override: %v", err)
	}
	if len(f.urls) != 1 || !strings.Contains(f.urls[0], "bing.com") {
		t.Fatalf("the override should have driven bing, drove %v", f.urls)
	}

	// An unknown name must fall back to the default rather than inventing an
	// engine or erroring on a config typo.
	odd := NewBrowserService(nil, "not-an-engine")
	if odd.Engine() != BrowserDDG {
		t.Fatalf("unknown engine should fall back to ddg, got %q", odd.Engine())
	}
}

// TestSkipURLCases: the schemes and hosts that are never a result.
func TestSkipURLCases(t *testing.T) {
	cases := []struct {
		url  string
		skip bool
	}{
		{"https://svelte.dev/docs", false},
		{"javascript:void(0)", true},
		{"mailto:someone@example.com", true},
		{"https://www.google.com/preferences", true},
		{"https://duckduckgo.com/y.js?ad=1", true},
		{"https://www.bing.com/images", true},
		{"https://www.google.com/policies/privacy", true},
	}
	for _, tc := range cases {
		if got := skipURL(tc.url); got != tc.skip {
			t.Errorf("skipURL(%q) = %v, want %v", tc.url, got, tc.skip)
		}
	}
}

// TestBrowserEngineNeedsAFetcher: without a bridge there is no browser, and the
// error has to say that rather than panicking on a nil interface.
func TestBrowserEngineNeedsAFetcher(t *testing.T) {
	svc := NewBrowserService(nil, BrowserDDG)
	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "x"})
	if err == nil || !strings.Contains(err.Error(), "browser fetcher") {
		t.Fatalf("want a 'needs a browser fetcher' error, got %v", err)
	}
}

// TestBrowserEngineRespectsTheLimit: asking for 3 must not return 30.
func TestBrowserEngineRespectsTheLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<html><body><ol>`)
	for i := 0; i < 6; i++ {
		b.WriteString(`<li class="b_algo"><h2><a href="https://example.org/`)
		b.WriteString(string(rune('a' + i)))
		b.WriteString(`">T</a></h2></li>`)
	}
	b.WriteString(`</ol></body></html>`)

	svc := NewBrowserService(&serpFetcher{html: b.String()}, BrowserBing)
	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 3})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
}

// TestChallengeIsJudgedByParsingNotByStrings is the regression test for a real
// false positive: DuckDuckGo and Google both ship the words "anomaly-modal" and
// "unusual traffic" in inline scripts on perfectly good result pages. A page
// that parses into results is not a challenge, whatever it says about itself.
func TestChallengeIsJudgedByParsingNotByStrings(t *testing.T) {
	// The DDG fixture with the challenge words pasted into an inline script.
	contaminated := strings.Replace(ddgHTML, "<body>",
		`<body><script>var t="anomaly-modal unusual traffic";</script>`, 1)

	svc := NewBrowserService(&serpFetcher{html: contaminated}, BrowserDDG)
	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 10})
	if err != nil {
		t.Fatalf("a page WITH results must not be refused as a challenge: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// And the same words on a page with no results still refuse.
	empty := `<html><body><script>var t="anomaly-modal";</script><p>no links here</p></body></html>`
	bad := NewBrowserService(&serpFetcher{html: empty}, BrowserDDG)
	if _, _, err := bad.Search(context.Background(), SearchOptions{Query: "x"}); !errors.Is(err, ErrBrowserChallenge) {
		t.Fatalf("want ErrBrowserChallenge when nothing parses, got %v", err)
	}
}

// TestBingStructuralFallback: the class names change, "a heading link inside the
// results list" does not.
func TestBingStructuralFallback(t *testing.T) {
	html := `<html><body><div id="b_results">
	  <div class="something-new"><h2><a href="https://example.org/a">A result</a></h2></div>
	</div></body></html>`

	svc := NewBrowserService(&serpFetcher{html: html}, BrowserBing)
	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://example.org/a" {
		t.Fatalf("structural fallback should have found the result, got %+v", results)
	}
}

// consentFetcher serves a consent wall first, then the results page, and records
// whether it was asked to drive the page.
type consentFetcher struct {
	wall     string
	results  string
	sawClick []FetchAction
	calls    int
}

func (f *consentFetcher) FetchHTML(_ context.Context, _ string) (string, error) {
	f.calls++
	if f.calls == 1 {
		return f.wall, nil
	}
	return f.results, nil
}

func (f *consentFetcher) FetchHTMLWithActions(_ context.Context, _ string, actions []FetchAction) (string, error) {
	f.calls++
	f.sawClick = actions
	return f.results, nil
}

const googleConsentWall = `<!doctype html><html><head><title>Before you continue to Google</title></head>
<body><div>We use cookies and data to deliver our services.</div>
<button id="W0wltc" aria-label="Reject all">Reject all</button>
<form action="/consent/save"><button id="L2AGLb" aria-label="Accept all">Accept all</button></form>
</body></html>`

// TestGoogleConsentIsAnsweredThenRead is the behaviour the live run demanded: a
// consent wall cannot be read past, only answered. The engine asks the browser to
// click the explicit Accept-all control once, then reads the results. Without
// this, every Google search on a fresh profile ends at the wall.
func TestGoogleConsentIsAnsweredThenRead(t *testing.T) {
	f := &consentFetcher{wall: googleConsentWall, results: googleHTML}
	svc := NewBrowserService(f, BrowserGoogle)

	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "runes", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://svelte.dev/blog/runes" {
		t.Fatalf("expected the results after the wall, got %+v", results)
	}
	if len(f.sawClick) == 0 {
		t.Fatalf("the fetcher was never asked to drive the consent page")
	}
	// Only explicit, labelled controls may be named. A "click the other button"
	// selector would be a privacy decision made on someone else's behalf.
	//
	// The two id selectors are allowed by name because they are Google's own
	// Accept-all controls (#L2AGLb on the classic wall, jsname b3VHJd on the
	// newer one); every other click has to say "Accept all" in the selector
	// itself, so its intent is readable without this comment.
	knownAcceptIDs := []string{"#L2AGLb", "button[jsname='b3VHJd']"}
	for _, a := range f.sawClick {
		if a.Type != "click" {
			continue
		}
		ok := strings.Contains(a.Selector, "Accept all")
		for _, id := range knownAcceptIDs {
			if a.Selector == id {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("refusing to click a control whose intent is not readable: %q", a.Selector)
		}
	}
}

// TestConsentWallWithoutAnActionCapableFetcher: a read-only fetcher still has to
// fail honestly, naming what it saw, rather than claiming "no results".
func TestConsentWallWithoutAnActionCapableFetcher(t *testing.T) {
	svc := NewBrowserService(&serpFetcher{html: googleConsentWall}, BrowserGoogle)
	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 5})
	if err == nil {
		t.Fatalf("want an error, got results")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Before you continue") {
		t.Fatalf("the error should name the page it got: %q", msg)
	}
	if errors.Is(err, ErrBrowserChallenge) {
		t.Fatalf("a consent wall is not a bot challenge: %q", msg)
	}
}

// TestGoogleFindsResultsOutsideRso is the live-run regression: with the Web
// filter the results container is not always `#rso`, and refusing a page whose
// title is literally "<query> - Google Search" was a false negative.
func TestGoogleFindsResultsOutsideRso(t *testing.T) {
	html := `<html><body><div id="main">
	  <div><a href="https://svelte.dev/blog/runes"><h3>Introducing runes</h3></a></div>
	  <div><a href="https://example.org/x"><h3>Another result</h3></a></div>
	</div></body></html>`

	svc := NewBrowserService(&serpFetcher{html: html}, BrowserGoogle)
	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(results), results)
	}
}

// TestBingRedirectIsUnwrapped: Bing reports results through its own click
// tracker, so the visible URL and the href disagree. Leaving the wrapper means
// dropping every result as an engine self-link (which is what happened live), or
// fetching the tracker instead of the page.
func TestBingRedirectIsUnwrapped(t *testing.T) {
	// "a1" + base64url("https://www.bluetooth.com")
	wrapped := "//www.bing.com/ck/a?!&&p=abc&u=a1aHR0cHM6Ly93d3cuYmx1ZXRvb3RoLmNvbQ&ntb=1"
	if got := unwrapRedirect(wrapped); got != "https://www.bluetooth.com" {
		t.Fatalf("unwrapRedirect = %q, want https://www.bluetooth.com", got)
	}
	if skipURL("https://www.bluetooth.com") {
		t.Fatalf("the unwrapped target must survive the skip list")
	}
	// A format change degrades to leaving the link alone, never to a wrong URL.
	if got := decodeBingTarget("garbage"); got != "" {
		t.Fatalf("decodeBingTarget(garbage) = %q, want empty", got)
	}
}
