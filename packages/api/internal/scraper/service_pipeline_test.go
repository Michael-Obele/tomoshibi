package scraper

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// countingScraper records how many times it ran, so tests can assert which
// engine was used rather than inferring it from the result alone. That
// distinction matters for the smart-mode retry rules, where "returned the
// static result" and "never tried the browser" are different claims.
type countingScraper struct {
	result *domain.ScrapeResult
	err    error
	calls  atomic.Int32
}

func (c *countingScraper) Scrape(context.Context, string, domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	c.calls.Add(1)
	if c.err != nil {
		return nil, c.err
	}
	return c.result, nil
}

// spaShellResult is a static result the heuristics classify as an unrendered
// SPA shell, which is what triggers the smart-mode dynamic retry.
func spaShellResult() *domain.ScrapeResult {
	return &domain.ScrapeResult{
		URL:      "https://spa.example.com",
		HTML:     `<html><body><div id="root"></div><script src="bundle.js"></script></body></html>`,
		Metadata: map[string]string{"engine": "colly"},
	}
}

// TestScrape_ActionsForceDynamic covers the mode override for page actions.
// Actions need a real browser, so every mode except an explicit static is
// promoted, and static is a hard error rather than a silent downgrade.
func TestScrape_ActionsForceDynamic(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantEngine string
		wantErr    string
	}{
		{name: "empty mode is promoted", mode: "", wantEngine: "chromedp"},
		{name: "smart is promoted", mode: "smart", wantEngine: "chromedp"},
		{name: "dynamic is left alone", mode: "dynamic", wantEngine: "chromedp"},
		{name: "static is rejected", mode: "static", wantErr: "actions require dynamic mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			colly := &countingScraper{result: newMockResult("colly")}
			chromedp := &countingScraper{result: newMockResult("chromedp")}
			svc := NewService(colly, chromedp, nil)

			result, err := svc.Scrape(context.Background(), "https://example.com", tt.mode, domain.ScrapeOptions{
				Actions: []domain.Action{{Type: "click", Selector: "#more"}},
			})

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %v, want it to mention %q", err, tt.wantErr)
				}
				// The mode conflict is a request-shape problem, so it must be
				// caught before an engine is started.
				if n := colly.calls.Load() + chromedp.calls.Load(); n != 0 {
					t.Errorf("an engine ran %d time(s) for a rejected request", n)
				}
				return
			}

			if err != nil {
				t.Fatalf("Scrape returned an error: %v", err)
			}
			if result.Metadata["engine"] != tt.wantEngine {
				t.Errorf("engine = %q, want %q", result.Metadata["engine"], tt.wantEngine)
			}
			if n := colly.calls.Load(); n != 0 {
				t.Errorf("the static engine ran %d time(s) despite actions", n)
			}
		})
	}
}

// TestScrape_SmartScreenshotGoesStraightToDynamic asserts smart mode does not
// waste a static fetch when a screenshot is requested — only a browser can
// produce one.
//
// The pipeline still runs after the browser capture, so enrichment applies to
// screenshot requests too; only the cache write is skipped, because the blob
// is large and every capture is time-sensitive.
func TestScrape_SmartScreenshotGoesStraightToDynamic(t *testing.T) {
	colly := &countingScraper{result: newMockResult("colly")}
	chromedp := &countingScraper{result: newMockResult("chromedp")}
	cache := newFakeCache()
	svc := withCache(colly, chromedp, cache)

	result, err := svc.Scrape(context.Background(), "https://example.com", "smart", domain.ScrapeOptions{
		Screenshot: true,
		ExtractSchema: map[string]domain.ExtractField{
			"headline": {Selector: "h1"},
		},
	})
	if err != nil {
		t.Fatalf("Scrape returned an error: %v", err)
	}
	if result.Metadata["engine"] != "chromedp" {
		t.Errorf("engine = %q, want chromedp", result.Metadata["engine"])
	}
	if n := colly.calls.Load(); n != 0 {
		t.Errorf("the static engine ran %d time(s) before the screenshot", n)
	}
	if result.Extracted["headline"] != "Example" {
		t.Errorf("schema extraction did not run for a screenshot request: extracted = %v", result.Extracted)
	}
	if cache.sets != 0 {
		t.Errorf("cache writes = %d; screenshot results are not cached", cache.sets)
	}
}

// TestScrape_SmartRetryPolicy covers which static failures earn a second
// attempt in the browser. Retrying costs a tab and several seconds, so only
// failures a JS runtime plausibly fixes qualify; DNS and connection errors
// fail identically in Chrome and must not be retried.
func TestScrape_SmartRetryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		staticErr error
		wantRetry bool
	}{
		{
			name:      "403 is a bot block a browser may pass",
			staticErr: &StatusError{StatusCode: http.StatusForbidden, Err: errors.New("blocked")},
			wantRetry: true,
		},
		{
			name:      "503 challenge page is retried",
			staticErr: &StatusError{StatusCode: http.StatusServiceUnavailable, Err: errors.New("challenge")},
			wantRetry: true,
		},
		{
			name:      "404 is permanent",
			staticErr: &StatusError{StatusCode: http.StatusNotFound, Err: errors.New("missing")},
			wantRetry: false,
		},
		{
			// 503 is retried but 500 is not: a challenge page is a 503, a
			// genuinely broken origin is a 500 and breaks the same way twice.
			name:      "500 is not retried",
			staticErr: &StatusError{StatusCode: http.StatusInternalServerError, Err: errors.New("boom")},
			wantRetry: false,
		},
		{
			name:      "an empty body may be a script-injected shell",
			staticErr: errors.New("colly: empty response"),
			wantRetry: true,
		},
		{
			name:      "a connection failure fails the same way in Chrome",
			staticErr: errors.New("dial tcp 10.0.0.1:80: connect: connection refused"),
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			colly := &countingScraper{err: tt.staticErr}
			chromedp := &countingScraper{result: newMockResult("chromedp")}
			svc := NewService(colly, chromedp, nil)

			result, err := svc.Scrape(context.Background(), "https://example.com", "smart", domain.ScrapeOptions{})

			if !tt.wantRetry {
				if err == nil {
					t.Fatal("expected the static error to be returned, got nil")
				}
				if n := chromedp.calls.Load(); n != 0 {
					t.Errorf("the browser ran %d time(s) for a non-retryable failure", n)
				}
				return
			}

			if err != nil {
				t.Fatalf("Scrape returned an error after a retryable failure: %v", err)
			}
			if result.Metadata["engine"] != "chromedp" {
				t.Errorf("engine = %q, want chromedp", result.Metadata["engine"])
			}
			if n := chromedp.calls.Load(); n != 1 {
				t.Errorf("the browser ran %d time(s), want exactly 1", n)
			}
		})
	}
}

// TestScrape_SmartBothEnginesFail asserts the static error surfaces when the
// speculative retry also fails. The static failure is the original cause; the
// dynamic one is an artefact of a retry the caller never asked for.
func TestScrape_SmartBothEnginesFail(t *testing.T) {
	colly := &countingScraper{err: &StatusError{StatusCode: http.StatusForbidden, Err: errors.New("cloudflare")}}
	chromedp := &countingScraper{err: errors.New("browser crashed")}
	svc := NewService(colly, chromedp, nil)

	_, err := svc.Scrape(context.Background(), "https://example.com", "smart", domain.ScrapeOptions{})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want the static 403 rather than the dynamic failure", err)
	}
	if strings.Contains(err.Error(), "browser crashed") {
		t.Errorf("error = %v, want the static cause, not the speculative retry's", err)
	}
}

// TestScrape_SmartFallsBackToThinStaticResult covers the case where static
// succeeded but looked like a shell and the browser then failed. A thin result
// is still a real one, so the request is answered rather than failed.
func TestScrape_SmartFallsBackToThinStaticResult(t *testing.T) {
	colly := &countingScraper{result: spaShellResult()}
	chromedp := &countingScraper{err: errors.New("no chromium available")}
	svc := NewService(colly, chromedp, nil)

	result, err := svc.Scrape(context.Background(), "https://spa.example.com", "smart", domain.ScrapeOptions{})
	if err != nil {
		t.Fatalf("Scrape returned an error instead of the static fallback: %v", err)
	}
	if result.Metadata["engine"] != "colly" {
		t.Errorf("engine = %q, want the static result", result.Metadata["engine"])
	}
	if n := chromedp.calls.Load(); n != 1 {
		t.Errorf("the browser ran %d time(s), want exactly 1", n)
	}
}
