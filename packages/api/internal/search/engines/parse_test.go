package engines

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	regOnce sync.Once
	regDef  *Registry
	regErr  error
)

func defaultReg(t *testing.T) *Registry {
	t.Helper()
	regOnce.Do(func() { regDef, regErr = LoadRegistry("") })
	if regErr != nil {
		t.Fatalf("load bundled registry: %v", regErr)
	}
	return regDef
}

// parseFixture runs a registry engine's parser over a captured live
// response (testdata/<engine>/<file>) — the anti-rot tripwire: when live
// re-capture differs, these assertions fail.
func parseFixture(t *testing.T, engine, file string) []Result {
	t.Helper()
	spec := defaultReg(t).Spec(engine)
	if spec == nil {
		t.Fatalf("engine %q missing from registry", engine)
	}
	body, err := os.ReadFile(filepath.Join("testdata", engine, file))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	res, err := parse(body, spec.Parse)
	if err != nil {
		t.Fatalf("parse %s fixture: %v", engine, err)
	}
	return res
}

func anyURL(res []Result, want string) bool {
	for _, r := range res {
		if r.URL == want {
			return true
		}
	}
	return false
}

func TestParseEngines(t *testing.T) {
	tests := []struct {
		engine  string
		file    string
		wantMin int
		check   func(t *testing.T, res []Result)
	}{
		{engine: "ddg", file: "web.html", wantMin: 8, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if strings.Contains(r.URL, "duckduckgo.com") {
					t.Errorf("uddg redirect not decoded: %s", r.URL)
				}
			}
			if !anyURL(res, "https://en.wikipedia.org/wiki/Web_scraping") {
				t.Error("expected the wikipedia hit (fixture contains it)")
			}
		}},
		{engine: "bing", file: "web.html", wantMin: 8, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if strings.Contains(r.URL, "bing.com/ck") {
					t.Errorf("ck/a redirect not decoded: %s", r.URL)
				}
				if !strings.HasPrefix(r.URL, "http") {
					t.Errorf("non-absolute result URL: %q", r.URL)
				}
			}
		}},
		{engine: "marginalia", file: "web.html", wantMin: 10, check: func(t *testing.T, res []Result) {
			if !anyURL(res, "https://en.wikipedia.org/wiki/Web_scraping") {
				t.Error("expected wikipedia as marginalia's first hit")
			}
		}},
		{engine: "wikipedia", file: "web.json", wantMin: 8, check: func(t *testing.T, res []Result) {
			if !anyURL(res, "https://en.wikipedia.org/wiki/Web%20scraping") {
				t.Errorf("wiki title not path-escaped; sample: %q", res[0].URL)
			}
			if res[0].Description == "" || strings.Contains(res[0].Description, "<span") {
				t.Errorf("snippet HTML not stripped: %q", res[0].Description)
			}
		}},
		{engine: "wikidata", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			if !strings.HasPrefix(res[0].URL, "https://www.wikidata.org/wiki/Q") {
				t.Errorf("bad wikidata URL %q", res[0].URL)
			}
			if res[0].Title == "" {
				t.Error("wikidata label empty (display.label.value path broken)")
			}
		}},
		{engine: "wikinews", file: "web.json", wantMin: 2, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if !strings.HasPrefix(r.URL, "https://en.wikinews.org/wiki/") {
					t.Errorf("bad wikinews URL %q", r.URL)
				}
			}
		}},
		{engine: "mwmbl", file: "web.json", wantMin: 10, check: func(t *testing.T, res []Result) {
			if res[0].URL == "" || res[0].Title == "" {
				t.Errorf("mwmbl join failed: %+v", res[0])
			}
			if !strings.Contains(res[0].Title, "Web") {
				t.Errorf("segmented title not joined: %q", res[0].Title)
			}
		}},
		{engine: "bingnews", file: "web.xml", wantMin: 5, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if strings.Contains(r.URL, "apiclick") {
					t.Errorf("apiclick redirect not decoded: %s", r.URL)
				}
			}
			dated := 0
			for _, r := range res {
				if r.Published != nil {
					dated++
				}
			}
			if dated == 0 {
				t.Error("no pubDate parsed from bing news RSS")
			}
		}},
		{engine: "hn", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if !strings.HasPrefix(r.URL, "http") {
					t.Errorf("bad hn URL %q (or_template fallback broken)", r.URL)
				}
			}
		}},
		{engine: "github", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if !strings.Contains(r.URL, "github.com/") {
					t.Errorf("bad github URL %q", r.URL)
				}
			}
		}},
		{engine: "stackexchange", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if !strings.Contains(r.URL, "stackoverflow.com") {
					t.Errorf("bad SE URL %q", r.URL)
				}
			}
		}},
		{engine: "mdn", file: "web.json", wantMin: 2, check: func(t *testing.T, res []Result) {
			if !strings.HasPrefix(res[0].URL, "https://developer.mozilla.org/") {
				t.Errorf("bad mdn URL %q", res[0].URL)
			}
		}},
		{engine: "npm", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			if !strings.HasPrefix(res[0].URL, "https://www.npmjs.com/package/") {
				t.Errorf("bad npm URL %q", res[0].URL)
			}
		}},
		{engine: "crates", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			if !strings.HasPrefix(res[0].URL, "https://crates.io/crates/") {
				t.Errorf("bad crates URL %q", res[0].URL)
			}
		}},
		{engine: "packagist", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			if !strings.HasPrefix(res[0].URL, "https://packagist.org/packages/") {
				t.Errorf("bad packagist URL %q", res[0].URL)
			}
		}},
		{engine: "brave", file: "web.json", wantMin: 5, check: func(t *testing.T, res []Result) {
			for _, r := range res {
				if !strings.HasPrefix(r.URL, "http") {
					t.Errorf("bad brave URL %q", r.URL)
				}
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.engine, func(t *testing.T) {
			res := parseFixture(t, tc.engine, tc.file)
			if len(res) < tc.wantMin {
				t.Fatalf("want >= %d results, got %d (first: %+v)", tc.wantMin, len(res), firstOrEmpty(res))
			}
			for i, r := range res {
				if r.Title == "" || r.URL == "" {
					t.Errorf("result %d missing title/url: %+v", i, r)
				}
			}
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

func firstOrEmpty(res []Result) Result {
	if len(res) == 0 {
		return Result{}
	}
	return res[0]
}

func TestDecodeRedirect(t *testing.T) {
	external := base64.RawStdEncoding.EncodeToString([]byte("https://example.com/x"))
	internal := base64.RawStdEncoding.EncodeToString([]byte("/images/search?q=test"))
	cases := []struct {
		name, in, kind, want string
	}{
		{"ddg", "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa%3Fb%3D1&rut=x", "uddg", "https://example.com/a?b=1"},
		{"google", "/url?q=https%3A%2F%2Fexample.com%2F&sa=U", "q", "https://example.com/"},
		{"bing news", "http://www.bing.com/news/apiclick.aspx?ref=FexRss&url=https%3a%2f%2fwww.forbes.com%2fx%2f&c=1", "url", "https://www.forbes.com/x/"},
		{"bing ck external", "https://www.bing.com/ck/a?p=1&u=a1" + external + "&ntb=1", "bing_u", "https://example.com/x"},
		{"bing ck internal", "https://www.bing.com/ck/a?p=1&u=a1" + internal + "&ntb=1", "bing_u", ""},
		{"direct passthrough", "https://example.com/direct", "uddg", "https://example.com/direct"},
		{"empty", "", "uddg", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeRedirect(tc.in, tc.kind)
			if got != tc.want {
				t.Errorf("decodeRedirect(%q,%s) = %q, want %q", tc.in, tc.kind, got, tc.want)
			}
		})
	}
}

func TestRenderURL(t *testing.T) {
	q := Query{Q: "web scraping & js", Pageno: 2, Language: "en", TimeRange: "week"}
	got := renderURL("https://x.test/s?q={{query}}&p={{pageno}}&l={{language}}&t={{time_range}}", q)
	want := "https://x.test/s?q=web+scraping+%26+js&p=2&l=en&t=week"
	if got != want {
		t.Errorf("renderURL = %q, want %q", got, want)
	}
	if out := renderURL("https://x.test/?k=${TEST_ENV_VAR_XYZ}", Query{}); out != "https://x.test/?k=" {
		t.Errorf("env expansion failed: %q", out)
	}
}

func TestStripHTML(t *testing.T) {
	got := stripHTML(`<span class="searchmatch">Web</span> scraping, or <b>web</b> harvest`)
	if got != "Web scraping, or web harvest" {
		t.Errorf("stripHTML = %q", got)
	}
}

func TestParseTimeLayouts(t *testing.T) {
	for _, s := range []string{
		"Tue, 22 Sep 2026 10:00:00 GMT",
		"Tue, 22 Sep 2026 10:00:00 +0000",
		"2026-09-22T10:00:00Z",
	} {
		if _, ok := parseTime(s); !ok {
			t.Errorf("parseTime(%q) failed", s)
		}
	}
	if _, ok := parseTime("not a date"); ok {
		t.Error("bogus date must not parse")
	}
	_ = time.Now
}
