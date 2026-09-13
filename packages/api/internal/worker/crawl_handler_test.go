package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

// =============================================================================
// Mock scraper — simulates real website pages with controllable link structure
// =============================================================================

// mockCrawlScraper provides predefined pages. URLs not in the map return errors,
// simulating real-world 404s or network failures.
type mockCrawlScraper struct {
	mu      sync.Mutex
	pages   map[string]*domain.ScrapeResult
	visited []string
}

func (m *mockCrawlScraper) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	m.mu.Lock()
	m.visited = append(m.visited, url)
	m.mu.Unlock()

	if result, ok := m.pages[url]; ok {
		return result, nil
	}
	return nil, fmt.Errorf("page not found: %s", url)
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// =============================================================================
// extractLinks — Real HTML patterns that crawlers encounter in production
// =============================================================================

func TestExtractLinks_RealisticNavigation(t *testing.T) {
	// Simulates a real website with header nav, content links, and footer.
	// This is the kind of HTML a crawler actually encounters.
	html := `<!DOCTYPE html>
	<html>
	<head><title>Docs</title></head>
	<body>
		<nav>
			<a href="/">Home</a>
			<a href="/docs">Docs</a>
			<a href="/docs/getting-started">Getting Started</a>
			<a href="/pricing">Pricing</a>
		</nav>
		<main>
			<h1>Welcome</h1>
			<p>Read the <a href="/docs/api-reference">API Reference</a>.</p>
			<p>Check out <a href="https://github.com/example/repo">our GitHub</a>.</p>
			<p>Download the <a href="/assets/sdk.zip">SDK</a>.</p>
		</main>
		<footer>
			<a href="/privacy">Privacy</a>
			<a href="/terms">Terms</a>
			<a href="https://twitter.com/example">Twitter</a>
		</footer>
	</body>
	</html>`

	links := extractLinks(html, "https://example.com", "example.com")

	// Should include same-domain pages:
	//   /, /docs, /docs/getting-started, /pricing, /docs/api-reference, /privacy, /terms
	// Should exclude:
	//   github.com (external), twitter.com (external), /assets/sdk.zip (resource)
	expectedLinks := map[string]bool{
		"https://example.com/":                     true, // href="/" resolves with trailing slash
		"https://example.com/docs":                 true,
		"https://example.com/docs/getting-started": true,
		"https://example.com/pricing":              true,
		"https://example.com/docs/api-reference":   true,
		"https://example.com/privacy":              true,
		"https://example.com/terms":                true,
	}

	if len(links) != len(expectedLinks) {
		t.Fatalf("Expected %d links, got %d: %v", len(expectedLinks), len(links), links)
	}
	for _, link := range links {
		if !expectedLinks[link] {
			t.Errorf("Unexpected link extracted: %q", link)
		}
	}
}

func TestExtractLinks_ProtocolRelativeURLs(t *testing.T) {
	// Protocol-relative URLs (//cdn.example.com) are common on real websites.
	html := `<html><body>
		<a href="//cdn.example.com/resource">CDN (subdomain)</a>
		<a href="//example.com/page">Same domain protocol-relative</a>
		<a href="//other.com/page">External protocol-relative</a>
	</body></html>`

	links := extractLinks(html, "https://example.com", "example.com")

	found := false
	for _, link := range links {
		if link == "https://example.com/page" {
			found = true
		}
		if link == "https://cdn.example.com/resource" || link == "https://other.com/page" {
			t.Errorf("Should not include external/subdomain link: %q", link)
		}
	}
	if !found {
		t.Errorf("Expected protocol-relative same-domain link to be included, got: %v", links)
	}
}

func TestExtractLinks_SubdomainFiltering(t *testing.T) {
	// Exact hostname match: docs.example.com should NOT follow blog.example.com
	html := `<html><body>
		<a href="https://docs.example.com/api">API Docs</a>
		<a href="https://blog.example.com/post">Blog Post</a>
		<a href="https://example.com/home">Main Site</a>
		<a href="/internal">Internal Link</a>
	</body></html>`

	links := extractLinks(html, "https://docs.example.com/intro", "docs.example.com")

	for _, link := range links {
		if link == "https://blog.example.com/post" || link == "https://example.com/home" {
			t.Errorf("Should not include cross-subdomain link: %q", link)
		}
	}

	hasInternal, hasAPI := false, false
	for _, link := range links {
		if link == "https://docs.example.com/internal" {
			hasInternal = true
		}
		if link == "https://docs.example.com/api" {
			hasAPI = true
		}
	}
	if !hasInternal {
		t.Error("Expected /internal to resolve to docs.example.com/internal")
	}
	if !hasAPI {
		t.Error("Expected API link to be included")
	}
}

func TestExtractLinks_SkipsNonHTTPSchemes(t *testing.T) {
	html := `<html><body>
		<a href="mailto:user@example.com">Email</a>
		<a href="tel:+1234567890">Phone</a>
		<a href="javascript:void(0)">JS</a>
		<a href="#">Anchor</a>
		<a href="/valid">Valid</a>
	</body></html>`

	links := extractLinks(html, "https://example.com", "example.com")

	if len(links) != 1 {
		t.Fatalf("Expected 1 valid link, got %d: %v", len(links), links)
	}
	if links[0] != "https://example.com/valid" {
		t.Errorf("Expected /valid, got %q", links[0])
	}
}

func TestExtractLinks_SkipsResourceFiles(t *testing.T) {
	html := `<html><body>
		<a href="/download.pdf">PDF</a>
		<a href="/image.jpg">Image</a>
		<a href="/style.css">CSS</a>
		<a href="/script.js">JS</a>
		<a href="/page">Valid page</a>
		<a href="/docs/api">API docs</a>
	</body></html>`

	links := extractLinks(html, "https://example.com", "example.com")

	if len(links) != 2 {
		t.Fatalf("Expected 2 non-resource links, got %d: %v", len(links), links)
	}
}

func TestExtractLinks_DeduplicatesFragmentVariants(t *testing.T) {
	// Same page linked multiple times with different fragments
	html := `<html><body>
		<a href="/page#section1">Section 1</a>
		<a href="/page#section2">Section 2</a>
		<a href="/page">No fragment</a>
	</body></html>`

	links := extractLinks(html, "https://example.com", "example.com")

	if len(links) != 1 {
		t.Fatalf("Expected 1 deduplicated link (fragments stripped), got %d: %v", len(links), links)
	}
	if links[0] != "https://example.com/page" {
		t.Errorf("Expected fragment-stripped URL, got %q", links[0])
	}
}

func TestExtractLinks_QueryParamVariationsAreDifferentPages(t *testing.T) {
	// Pagination links — different query params = different pages
	html := `<html><body>
		<a href="/search?page=1">Page 1</a>
		<a href="/search?page=2">Page 2</a>
		<a href="/search?page=1">Duplicate</a>
	</body></html>`

	links := extractLinks(html, "https://example.com", "example.com")

	if len(links) != 2 {
		t.Fatalf("Expected 2 unique links (different query params), got %d: %v", len(links), links)
	}
}

func TestExtractLinks_ResolvesRelativeURLs(t *testing.T) {
	html := `<html><body>
		<a href="sibling">Sibling</a>
		<a href="../parent">Parent</a>
		<a href="./current">Current</a>
	</body></html>`

	links := extractLinks(html, "https://example.com/docs/intro", "example.com")

	expected := map[string]bool{
		"https://example.com/docs/sibling": true,
		"https://example.com/parent":       true,
		"https://example.com/docs/current": true,
	}

	if len(links) != 3 {
		t.Fatalf("Expected 3 resolved links, got %d: %v", len(links), links)
	}
	for _, link := range links {
		if !expected[link] {
			t.Errorf("Unexpected resolved link: %q", link)
		}
	}
}

func TestExtractLinks_EmptyHTML(t *testing.T) {
	links := extractLinks("", "https://example.com", "example.com")
	if links != nil {
		t.Errorf("Expected nil for empty HTML, got %v", links)
	}
}

func TestExtractLinks_SameDomainDifferentPort(t *testing.T) {
	// Go's url.Hostname() strips the port, so example.com:8080 has hostname "example.com".
	// This means URLs with a different port on the same hostname WILL pass domain filtering.
	// This is acceptable behavior — the crawler treats all ports on a hostname as same-domain.
	html := `<html><body>
		<a href="https://example.com:8080/api">Different port</a>
		<a href="https://example.com/page">Same host</a>
	</body></html>`

	links := extractLinks(html, "https://example.com", "example.com")

	// Both should be included since Go resolves hostname without port
	if len(links) != 2 {
		t.Errorf("Expected 2 links (Go strips port from hostname), got %d: %v", len(links), links)
	}
}

func TestExtractLinks_EncodedURLs(t *testing.T) {
	html := `<html><body>
		<a href="/search?q=hello%20world">Search</a>
		<a href="/caf%C3%A9">Café page</a>
	</body></html>`

	links := extractLinks(html, "https://example.com", "example.com")

	if len(links) != 2 {
		t.Fatalf("Expected 2 links with encoded chars, got %d: %v", len(links), links)
	}
}

// =============================================================================
// normalizeURL
// =============================================================================

func TestNormalizeURL_StripsFragment(t *testing.T) {
	if got := normalizeURL("https://example.com/page#section"); got != "https://example.com/page" {
		t.Errorf("got %q, want fragment stripped", got)
	}
}

func TestNormalizeURL_StripsTrailingSlash(t *testing.T) {
	if got := normalizeURL("https://example.com/page/"); got != "https://example.com/page" {
		t.Errorf("got %q, want trailing slash stripped", got)
	}
}

func TestNormalizeURL_PreservesQueryParams(t *testing.T) {
	if got := normalizeURL("https://example.com/page?foo=bar&baz=1"); got != "https://example.com/page?foo=bar&baz=1" {
		t.Errorf("got %q, want query params preserved", got)
	}
}

func TestNormalizeURL_CombinedFragmentAndTrailingSlash(t *testing.T) {
	if got := normalizeURL("https://example.com/docs/#intro"); got != "https://example.com/docs" {
		t.Errorf("got %q, want both fragment and trailing slash stripped", got)
	}
}

func TestNormalizeURL_InvalidURL(t *testing.T) {
	if got := normalizeURL(":::invalid"); got != ":::invalid" {
		t.Errorf("should return input verbatim for invalid URLs, got %q", got)
	}
}

// =============================================================================
// isResourceFile
// =============================================================================

func TestIsResourceFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/download.pdf", true},
		{"/photo.jpg", true},
		{"/photo.JPEG", true},
		{"/icon.png", true},
		{"/style.css", true},
		{"/app.js", true},
		{"/font.woff2", true},
		{"/archive.zip", true},
		{"/feed.xml", true},
		{"/video.mp4", true},

		// Crawlable pages — these should NOT be filtered
		{"/docs/getting-started", false},
		{"/api/v1/users", false},
		{"/", false},
		{"", false},
		{"/page.html", false},
		{"/page.htm", false},
		{"/page.php", false},
		{"/page.aspx", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isResourceFile(tt.path); got != tt.expected {
				t.Errorf("isResourceFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

// =============================================================================
// ExecuteCrawl — End-to-end BFS tests using mock scraper
// These test real crawl behavior: depth traversal, limits, dedup, failures.
// =============================================================================

func TestExecuteCrawl_BFS_DepthTracking(t *testing.T) {
	// 3-level site: / → /about, /docs → /about/team, /docs/api
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML:     `<html><body><a href="/about">About</a><a href="/docs">Docs</a></body></html>`,
				Metadata: map[string]string{"engine": "mock"},
			},
			"https://example.com/about": {
				URL: "https://example.com/about", Markdown: "# About",
				HTML:     `<html><body><a href="/about/team">Team</a></body></html>`,
				Metadata: map[string]string{"engine": "mock"},
			},
			"https://example.com/docs": {
				URL: "https://example.com/docs", Markdown: "# Docs",
				HTML:     `<html><body><a href="/docs/api">API</a></body></html>`,
				Metadata: map[string]string{"engine": "mock"},
			},
			"https://example.com/about/team": {
				URL: "https://example.com/about/team", Markdown: "# Team",
				HTML: `<html><body><p>End of branch</p></body></html>`, Metadata: map[string]string{},
			},
			"https://example.com/docs/api": {
				URL: "https://example.com/docs/api", Markdown: "# API",
				HTML: `<html><body><p>End of branch</p></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, err := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 2, Limit: 100,
	}, "test-task-1")

	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}

	// Depth 0: / (1), Depth 1: /about, /docs (2), Depth 2: /about/team, /docs/api (2) = 5
	if result.TotalPages != 5 {
		t.Errorf("Expected 5 pages, got %d", result.TotalPages)
	}
	if result.Status != "completed" {
		t.Errorf("Expected 'completed', got %q", result.Status)
	}
}

func TestExecuteCrawl_RespectsMaxDepth(t *testing.T) {
	// Linear deep site, maxDepth=1 should only crawl seed + direct children
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: `<html><body><a href="/level1">L1</a></body></html>`, Metadata: map[string]string{},
			},
			"https://example.com/level1": {
				URL: "https://example.com/level1", Markdown: "# L1",
				HTML: `<html><body><a href="/level1/level2">L2</a></body></html>`, Metadata: map[string]string{},
			},
			"https://example.com/level1/level2": {
				URL: "https://example.com/level1/level2", Markdown: "# L2",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 1, Limit: 100,
	}, "test-depth")

	// maxDepth=1: seed (depth 0) + /level1 (depth 1) = 2 pages. /level1/level2 is depth 2 → skipped.
	if result.TotalPages != 2 {
		t.Errorf("maxDepth=1: expected 2 pages, got %d", result.TotalPages)
	}
}

func TestExecuteCrawl_RespectsLimit(t *testing.T) {
	// Wide site with many pages, limit=3
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: `<html><body>
					<a href="/p1">1</a><a href="/p2">2</a><a href="/p3">3</a>
					<a href="/p4">4</a><a href="/p5">5</a>
				</body></html>`, Metadata: map[string]string{},
			},
			"https://example.com/p1": {URL: "https://example.com/p1", Markdown: "P1", HTML: "<html></html>", Metadata: map[string]string{}},
			"https://example.com/p2": {URL: "https://example.com/p2", Markdown: "P2", HTML: "<html></html>", Metadata: map[string]string{}},
			"https://example.com/p3": {URL: "https://example.com/p3", Markdown: "P3", HTML: "<html></html>", Metadata: map[string]string{}},
			"https://example.com/p4": {URL: "https://example.com/p4", Markdown: "P4", HTML: "<html></html>", Metadata: map[string]string{}},
			"https://example.com/p5": {URL: "https://example.com/p5", Markdown: "P5", HTML: "<html></html>", Metadata: map[string]string{}},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 5, Limit: 3,
	}, "test-limit")

	if result.TotalPages > 3 {
		t.Errorf("Limit=3 but crawled %d pages", result.TotalPages)
	}
}

func TestExecuteCrawl_CircularLinks(t *testing.T) {
	// Pages link back to each other — must NOT infinite-loop.
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML:     `<html><body><a href="/about">About</a></body></html>`,
				Metadata: map[string]string{},
			},
			"https://example.com/about": {
				URL: "https://example.com/about", Markdown: "# About",
				// Links back to home and to itself
				HTML:     `<html><body><a href="/">Home</a><a href="/about">Self</a></body></html>`,
				Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, err := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 10, Limit: 100,
	}, "test-circular")

	if err != nil {
		t.Fatalf("Should not fail on circular links: %v", err)
	}

	if result.TotalPages != 2 {
		t.Errorf("Circular links: expected exactly 2 unique pages, got %d", result.TotalPages)
	}
	if result.Status != "completed" {
		t.Errorf("Expected 'completed' (no failures), got %q", result.Status)
	}
}

func TestExecuteCrawl_PartialFailures(t *testing.T) {
	// /broken is NOT in the mock → scrape returns error
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML:     `<html><body><a href="/good">Good</a><a href="/broken">Broken</a></body></html>`,
				Metadata: map[string]string{},
			},
			"https://example.com/good": {
				URL: "https://example.com/good", Markdown: "# Good Page",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 2, Limit: 100,
	}, "test-partial")

	if result.TotalPages != 2 {
		t.Errorf("Expected 2 successful pages, got %d", result.TotalPages)
	}
	if result.Status != "partial" {
		t.Errorf("Expected 'partial' (some failures), got %q", result.Status)
	}
	if len(result.FailedURLs) != 1 {
		t.Fatalf("Expected 1 failed URL, got %d", len(result.FailedURLs))
	}
	if result.FailedURLs[0].URL != "https://example.com/broken" {
		t.Errorf("Expected failed URL 'https://example.com/broken', got %q", result.FailedURLs[0].URL)
	}
}

func TestExecuteCrawl_AllFailures(t *testing.T) {
	// Seed URL itself fails → status should be "failed"
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{}, // No pages at all
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 2, Limit: 10,
	}, "test-allfail")

	if result.TotalPages != 0 {
		t.Errorf("Expected 0 pages, got %d", result.TotalPages)
	}
	if result.Status != "failed" {
		t.Errorf("Expected 'failed' when seed URL fails, got %q", result.Status)
	}
}

func TestExecuteCrawl_ExternalLinksNeverFollowed(t *testing.T) {
	// Verify that external links embedded in page HTML are never crawled
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://mysite.com": {
				URL: "https://mysite.com", Markdown: "# Home",
				HTML: `<html><body>
					<a href="/about">About</a>
					<a href="https://evil.com/phish">Phishing</a>
					<a href="https://google.com">Google</a>
				</body></html>`, Metadata: map[string]string{},
			},
			"https://mysite.com/about": {
				URL: "https://mysite.com/about", Markdown: "# About",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
			// These should never be reached:
			"https://evil.com/phish": {
				URL: "https://evil.com/phish", Markdown: "# EVIL",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://mysite.com", MaxDepth: 5, Limit: 100,
	}, "test-external")

	if result.TotalPages != 2 {
		t.Errorf("Expected 2 pages (only same-domain), got %d", result.TotalPages)
	}
	for _, page := range result.Pages {
		if page.URL != "https://mysite.com" && page.URL != "https://mysite.com/about" {
			t.Errorf("Crawled external domain: %q", page.URL)
		}
	}
}

func TestExecuteCrawl_DefaultsCapping(t *testing.T) {
	// maxDepth=0, limit=0 → should apply defaults of 2 and 10
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 0, Limit: 0,
	}, "test-defaults")

	if result.MaxDepth != 2 {
		t.Errorf("Expected default maxDepth=2, got %d", result.MaxDepth)
	}
	if result.Limit != 10 {
		t.Errorf("Expected default limit=10, got %d", result.Limit)
	}
}

func TestExecuteCrawl_OverMaxCapping(t *testing.T) {
	// maxDepth=99, limit=9999 → should cap to 10 and 100
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 99, Limit: 9999,
	}, "test-caps")

	if result.MaxDepth != 10 {
		t.Errorf("Expected capped maxDepth=10, got %d", result.MaxDepth)
	}
	if result.Limit != 100 {
		t.Errorf("Expected capped limit=100, got %d", result.Limit)
	}
}

func TestExecuteCrawl_ContextCancellation(t *testing.T) {
	// Cancelled context should stop the crawl and return partial results
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: `<html><body><a href="/p1">P1</a></body></html>`, Metadata: map[string]string{},
			},
			"https://example.com/p1": {
				URL: "https://example.com/p1", Markdown: "P1",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, _ := handler.ExecuteCrawl(ctx, CrawlPayload{
		URL: "https://example.com", MaxDepth: 5, Limit: 100,
	}, "test-cancel")

	if result.Status != "cancelled" {
		t.Errorf("Expected 'cancelled' status, got %q", result.Status)
	}
}

func TestExecuteCrawl_ResourceLinksNotFollowed(t *testing.T) {
	// Page with mix of crawlable and resource links
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: `<html><body>
					<a href="/page">Valid page</a>
					<a href="/report.pdf">PDF Report</a>
					<a href="/logo.png">Logo</a>
					<a href="/styles.css">Styles</a>
					<a href="/data.json">Data</a>
				</body></html>`, Metadata: map[string]string{},
			},
			"https://example.com/page": {
				URL: "https://example.com/page", Markdown: "# Page",
				HTML: `<html><body></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	result, _ := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL: "https://example.com", MaxDepth: 5, Limit: 100,
	}, "test-resources")

	// Should only scrape / and /page — never attempt .pdf, .png, .css, .json
	if result.TotalPages != 2 {
		t.Errorf("Expected 2 pages (skipping resources), got %d", result.TotalPages)
	}
	if result.Status != "completed" {
		t.Errorf("Expected 'completed', got %q", result.Status)
	}

	// Verify the mock was never asked to scrape resource URLs
	for _, url := range mock.visited {
		if url == "https://example.com/report.pdf" ||
			url == "https://example.com/logo.png" ||
			url == "https://example.com/styles.css" ||
			url == "https://example.com/data.json" {
			t.Errorf("Should not have attempted to scrape resource URL: %q", url)
		}
	}
}

// TestExecuteCrawl_WidePageNoDeadlock is a regression test for the
// producer–consumer deadlock: a single page yielding more links than the
// queue buffer (8) used to block every worker in `queue <- e` until the
// crawl hung forever. Enqueue must be non-blocking — excess links are
// dropped and the crawl completes with at most `limit` pages.
func TestExecuteCrawl_WidePageNoDeadlock(t *testing.T) {
	links := make([]string, 0, 300)
	for i := 0; i < 300; i++ {
		links = append(links, fmt.Sprintf(`<a href="/page-%d">P%d</a>`, i, i))
	}
	seedHTML := "<html><body>" + strings.Join(links, "") + "</body></html>"

	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: seedHTML, Metadata: map[string]string{},
			},
		},
	}
	// Every discovered page resolves in the mock (returns a page-not-found
	// error otherwise — which is fine; the crawl must still terminate).
	for i := 0; i < 300; i++ {
		u := fmt.Sprintf("https://example.com/page-%d", i)
		mock.pages[u] = &domain.ScrapeResult{URL: u, Markdown: "P", HTML: "<html></html>", Metadata: map[string]string{}}
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	// If the deadlock regresses, ExecuteCrawl would still return once the
	// deadline fires — but with "timeout" instead of "completed".
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := handler.ExecuteCrawl(ctx, CrawlPayload{
		URL: "https://example.com", MaxDepth: 2, Limit: 100,
	}, "test-wide")

	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("Expected 'completed' (no deadlock), got %q", result.Status)
	}
	if result.TotalPages == 0 {
		t.Fatal("Expected at least the seed page to be scraped")
	}
	if result.TotalPages > result.Limit {
		t.Errorf("Queue-depth cap violated: %d pages scraped with limit %d", result.TotalPages, result.Limit)
	}
}

// TestExecuteCrawl_DeadlineExceeded verifies the internal watchdog maps a
// deadline expiry to the "timeout" status instead of hanging the worker.
func TestExecuteCrawl_DeadlineExceeded(t *testing.T) {
	mock := &mockCrawlScraper{
		pages: map[string]*domain.ScrapeResult{
			"https://example.com": {
				URL: "https://example.com", Markdown: "# Home",
				HTML: `<html><body><a href="/p1">P1</a></body></html>`, Metadata: map[string]string{},
			},
		},
	}

	svc := scraper.NewService(mock, nil, nil)
	handler := NewCrawlTaskHandler(svc, newTestLogger())

	// Pre-expired deadline → crawlCtx.Err() is DeadlineExceeded.
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()

	result, _ := handler.ExecuteCrawl(ctx, CrawlPayload{
		URL: "https://example.com", MaxDepth: 5, Limit: 100,
	}, "test-timeout")

	if result.Status != "timeout" {
		t.Errorf("Expected 'timeout' status, got %q", result.Status)
	}
}

// TestMatchesAny_DoubleStar verifies separator-aware glob matching with
// `**` support (path.Match cannot cross path segments, gobwas/glob can
// when compiled with '/' as the separator).
func TestMatchesAny_DoubleStar(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/blog/*", "/blog/post", true},
		{"/blog/*", "/blog/a/b/post", false}, // * does not cross /
		{"/blog/**", "/blog/a/b/post", true}, // ** crosses /
		{"/blog/**", "/blog/", true},
		{"/blog/**", "/blog", false}, // pattern carries a trailing slash
		{"/catalogue/*", "/catalogue/book", true},
		{"/docs/*.md", "/docs/api.md", true},
		{"/docs/?", "/docs/a", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" vs "+tt.path, func(t *testing.T) {
			if got := matchesAny([]string{tt.pattern}, tt.path); got != tt.want {
				t.Errorf("matchesAny(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
