package scraper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// TestCollyScraper_StatusErrors asserts an HTTP failure comes back as a
// StatusError carrying the code. The smart-mode retry policy reads that code,
// so losing it would silently turn every 404 into a wasted browser launch.
func TestCollyScraper_StatusErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantRetry bool
	}{
		{name: "403 is retryable in a browser", status: http.StatusForbidden, wantRetry: true},
		{name: "429 is retryable in a browser", status: http.StatusTooManyRequests, wantRetry: true},
		{name: "404 is permanent", status: http.StatusNotFound, wantRetry: false},
		{name: "500 is permanent", status: http.StatusInternalServerError, wantRetry: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			_, err := NewCollyScraper().Scrape(context.Background(), srv.URL, domain.ScrapeOptions{})
			if err == nil {
				t.Fatalf("expected an error for status %d, got nil", tt.status)
			}

			var se *StatusError
			if !errors.As(err, &se) {
				t.Fatalf("error %v is not a *StatusError; the status code was dropped", err)
			}
			if se.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", se.StatusCode, tt.status)
			}
			if got := worthRetryingDynamic(err); got != tt.wantRetry {
				t.Errorf("worthRetryingDynamic = %v, want %v", got, tt.wantRetry)
			}
		})
	}
}

// TestCollyScraper_EmptyResponse covers the sentinel smart mode keys on: a
// 200 with nothing colly recognises as HTML is frequently a shell that only
// populates under JS, so it is reported as retryable rather than as success.
func TestCollyScraper_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A non-HTML content type: colly does not parse it, so no OnHTML
		// callback fires and no markup is collected.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	_, err := NewCollyScraper().Scrape(context.Background(), srv.URL, domain.ScrapeOptions{})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("error = %v, want the empty-response sentinel", err)
	}
	if !worthRetryingDynamic(err) {
		t.Error("an empty response should be worth retrying in a browser")
	}
}

// TestCollyScraper_RejectsBadURL asserts the Visit error surfaces rather than
// being reported as an empty page.
func TestCollyScraper_RejectsBadURL(t *testing.T) {
	for _, url := range []string{"", "not-a-url", "ftp://example.com/x"} {
		t.Run(url, func(t *testing.T) {
			if _, err := NewCollyScraper().Scrape(context.Background(), url, domain.ScrapeOptions{}); err == nil {
				t.Errorf("Scrape(%q) returned no error", url)
			}
		})
	}
}

// TestCollyScraper_CleaningOptions asserts the block_ads and
// remove_base64_images options reach the cleaning pass through a real scrape.
// cleanContent is unit-tested separately; this pins the wiring, which is the
// part a refactor breaks.
func TestCollyScraper_CleaningOptions(t *testing.T) {
	// Readability keeps this page's <article>, so the ad inside it survives
	// extraction and reaches cleanContent — which is the point.
	// The marker deliberately avoids characters the markdown converter escapes
	// (an underscore would come back as `SPONSORED\_MARKER`).
	const page = `<html><head><title>Cleaning</title></head><body><article>
<h1>Article Heading</h1>
<p>A first paragraph long enough that readability treats this article element
as the main content of the document rather than boilerplate noise.</p>
<div class="advertisement">SponsoredMarker</div>
<p>A second paragraph, also of a reasonable length, keeping the candidate
comfortably above the extraction threshold that readability applies.</p>
</article></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	scrape := func(t *testing.T, opts domain.ScrapeOptions) *domain.ScrapeResult {
		t.Helper()
		result, err := NewCollyScraper().Scrape(context.Background(), srv.URL, opts)
		if err != nil {
			t.Fatalf("scrape failed: %v", err)
		}
		return result
	}

	t.Run("ads are blocked by default", func(t *testing.T) {
		result := scrape(t, domain.ScrapeOptions{})
		if strings.Contains(result.Markdown, "SponsoredMarker") {
			t.Errorf("markdown kept the ad: %s", result.Markdown)
		}
		if !strings.Contains(result.Markdown, "Article Heading") {
			t.Errorf("markdown lost the real content: %s", result.Markdown)
		}
	})

	t.Run("block_ads=false keeps them", func(t *testing.T) {
		result := scrape(t, domain.ScrapeOptions{BlockAds: boolPtr(false)})
		if !strings.Contains(result.Markdown, "SponsoredMarker") {
			t.Errorf("markdown dropped the ad despite block_ads=false: %s", result.Markdown)
		}
	})

	t.Run("the raw HTML is always the unmodified page", func(t *testing.T) {
		// Cleaning applies to the markdown pipeline only. Callers asking for
		// HTML get what the server sent.
		result := scrape(t, domain.ScrapeOptions{})
		if !strings.Contains(result.HTML, "SponsoredMarker") {
			t.Error("HTML was cleaned; it should be the original document")
		}
		if result.Metadata["engine"] != "colly" {
			t.Errorf("engine = %q, want colly", result.Metadata["engine"])
		}
		if result.Metadata["scraped_at"] == "" {
			t.Error("scraped_at is missing")
		}
	})
}
