package scraper

import (
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

// TestCleanContent covers the two output-cleaning passes. Both default on in// service.go, so the interesting cases are the removals themselves and the
// explicit opt-out — a caller who wants the raw page must still get it.
func TestCleanContent(t *testing.T) {
	const page = `<html><body>` +
		`<p>Real body text.</p>` +
		`<div class="ad">Buy now</div>` +
		`<div class="advertisement">Sponsored</div>` +
		`<div class="promo-advert">Native advert</div>` +
		`<iframe src="https://doubleclick.net/x"></iframe>` +
		`<div aria-label="Advertisement">Labelled ad</div>` +
		`<img src="data:image/png;base64,AAAA" alt="inline">` +
		`<img src="https://cdn.example.com/real.png" alt="real">` +
		`</body></html>`

	tests := []struct {
		name               string
		blockAds           bool
		removeBase64Images bool
		wantAbsent         []string
		wantPresent        []string
	}{
		{
			name:               "both passes on",
			blockAds:           true,
			removeBase64Images: true,
			wantAbsent: []string{
				"Buy now", "Sponsored", "Native advert",
				"doubleclick.net", "Labelled ad", "data:image/png",
			},
			wantPresent: []string{"Real body text.", "cdn.example.com/real.png"},
		},
		{
			name:               "ads kept when blocking is disabled",
			blockAds:           false,
			removeBase64Images: true,
			wantAbsent:         []string{"data:image/png"},
			wantPresent:        []string{"Buy now", "Sponsored", "doubleclick.net"},
		},
		{
			name:               "inline images kept when removal is disabled",
			blockAds:           true,
			removeBase64Images: false,
			wantAbsent:         []string{"Buy now"},
			wantPresent:        []string{"data:image/png"},
		},
		{
			name:               "both passes off returns the page intact",
			blockAds:           false,
			removeBase64Images: false,
			wantPresent:        []string{"Buy now", "data:image/png", "Real body text."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanContent(page, tt.blockAds, tt.removeBase64Images)

			for _, s := range tt.wantAbsent {
				if strings.Contains(got, s) {
					t.Errorf("output still contains %q", s)
				}
			}
			for _, s := range tt.wantPresent {
				if !strings.Contains(got, s) {
					t.Errorf("output is missing %q", s)
				}
			}
		})
	}
}

// TestApplyReadabilityMetadata asserts only non-empty fields are written.
// Writing empty strings would turn "readability found no byline" into "the
// byline is blank", which consumers cannot tell apart.
func TestApplyReadabilityMetadata(t *testing.T) {
	t.Run("copies populated fields", func(t *testing.T) {
		meta := map[string]string{"engine": "colly"}
		applyReadabilityMetadata(meta, &ReadabilityResult{
			Title:    "The Title",
			Excerpt:  "A summary.",
			Byline:   "A. Writer",
			SiteName: "Example",
		})

		for key, want := range map[string]string{
			"title":     "The Title",
			"excerpt":   "A summary.",
			"byline":    "A. Writer",
			"site_name": "Example",
			"engine":    "colly",
		} {
			if meta[key] != want {
				t.Errorf("meta[%q] = %q, want %q", key, meta[key], want)
			}
		}
	})

	t.Run("skips empty fields", func(t *testing.T) {
		meta := map[string]string{}
		applyReadabilityMetadata(meta, &ReadabilityResult{Title: "Only Title"})

		if len(meta) != 1 {
			t.Errorf("meta = %v, want only the title", meta)
		}
	})

	t.Run("tolerates a nil result", func(t *testing.T) {
		meta := map[string]string{"engine": "colly"}
		applyReadabilityMetadata(meta, nil)
		if len(meta) != 1 {
			t.Errorf("meta = %v, want it untouched", meta)
		}
	})
}

// TestMustParseURL covers the zero-URL fallback. Readability needs a base URL
// for relative-link resolution and a parse failure must not abort the scrape.
func TestMustParseURL(t *testing.T) {
	if got := mustParseURL("https://example.com/a/b"); got.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", got.Host)
	}
	// A control character in the host makes url.Parse fail.
	if got := mustParseURL("http://ho\x7fst"); got == nil || got.Host != "" {
		t.Errorf("expected a zero URL for an unparseable input, got %#v", got)
	}
}
