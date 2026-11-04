package scraper

import (
	"context"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestService_CaptureNetworkRequiresDynamicMode(t *testing.T) {
	svc := NewService(&countingScraper{result: newMockResult("colly")}, nil, nil)

	_, err := svc.Scrape(context.Background(), "https://example.com", "static", domain.ScrapeOptions{
		CaptureNetwork: true,
	})
	if err == nil {
		t.Fatal("expected an error for capture_network in static mode")
	}
}

func TestService_CaptureNetworkForcesDynamicInSmartMode(t *testing.T) {
	static := &countingScraper{result: newMockResult("colly")}
	dynamic := &countingScraper{result: newMockResult("chromedp")}
	svc := NewService(static, dynamic, nil)

	result, err := svc.Scrape(context.Background(), "https://example.com", "smart", domain.ScrapeOptions{
		CaptureNetwork: true,
	})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if got := static.calls.Load(); got != 0 {
		t.Fatalf("static scraper called %d times, want 0", got)
	}
	if got := dynamic.calls.Load(); got != 1 {
		t.Fatalf("dynamic scraper called %d times, want 1", got)
	}
	if result.Metadata["engine"] != "chromedp" {
		t.Fatalf("engine = %q, want chromedp", result.Metadata["engine"])
	}
}

func TestScrape_CaptureNetworkSkipsCacheWrite(t *testing.T) {
	cache := newFakeCache()
	svc := withCache(nil, &mockScraper{result: newMockResult("chromedp")}, cache)

	if _, err := svc.Scrape(context.Background(), "https://example.com", "dynamic", domain.ScrapeOptions{
		CaptureNetwork: true,
	}); err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if len(cache.data) != 0 {
		t.Fatalf("capture result was cached: %d entries", len(cache.data))
	}
}
