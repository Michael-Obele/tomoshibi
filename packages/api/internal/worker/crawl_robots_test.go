package worker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/robots"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

// TestExecuteCrawl_RespectsRobotsDisallow covers the policy the BFS tests opt
// out of: a link the origin disallows is never fetched, while the seed URL is
// always scraped (matching include/exclude semantics).
func TestExecuteCrawl_RespectsRobotsDisallow(t *testing.T) {
	t.Setenv("CRAWL_IGNORE_ROBOTS", "false")

	var robotsHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			robotsHits++
			fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	mock := &mockCrawlScraper{pages: map[string]*domain.ScrapeResult{
		srv.URL: {
			URL: srv.URL, Markdown: "# Home",
			HTML:     `<html><body><a href="/allowed">A</a><a href="/private">P</a></body></html>`,
			Metadata: map[string]string{},
		},
		srv.URL + "/allowed": {
			URL: srv.URL + "/allowed", Markdown: "# Allowed",
			HTML:     `<html><body></body></html>`,
			Metadata: map[string]string{},
		},
	}}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger()).
		WithRobotsCache(robots.NewCache(srv.Client()))

	result, err := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: srv.URL, MaxDepth: 2, Limit: 10,
	}, "test-robots")
	if err != nil {
		t.Fatalf("ExecuteCrawl: %v", err)
	}

	if result.TotalPages != 2 {
		t.Fatalf("crawled %d pages, want 2 (seed + allowed): %+v", result.TotalPages, result.Pages)
	}
	for _, page := range result.Pages {
		if strings.HasSuffix(page.URL, "/private") {
			t.Fatalf("crawled a disallowed URL: %s", page.URL)
		}
	}
	if robotsHits != 1 {
		t.Fatalf("robots.txt fetched %d times, want 1", robotsHits)
	}
}

// TestExecuteCrawl_SeedBypassesRobots keeps the documented exception honest:
// the caller asked for this URL explicitly, so a blanket Disallow cannot turn
// a crawl into a no-op.
func TestExecuteCrawl_SeedBypassesRobots(t *testing.T) {
	t.Setenv("CRAWL_IGNORE_ROBOTS", "false")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nDisallow: /\n")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	mock := &mockCrawlScraper{pages: map[string]*domain.ScrapeResult{
		srv.URL: {
			URL: srv.URL, Markdown: "# Home",
			HTML:     `<html><body></body></html>`,
			Metadata: map[string]string{},
		},
	}}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger()).
		WithRobotsCache(robots.NewCache(srv.Client()))

	result, err := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: srv.URL, MaxDepth: 1, Limit: 5,
	}, "test-robots-seed")
	if err != nil {
		t.Fatalf("ExecuteCrawl: %v", err)
	}
	if result.TotalPages != 1 {
		t.Fatalf("seed was not scraped: %+v", result)
	}
}
