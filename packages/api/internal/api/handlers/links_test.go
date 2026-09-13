package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

func TestScrapeHandler_LinksDefaultTrue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wantedLinks := []domain.LinkData{{URL: "https://iana.org/domains/example", Text: "More information...", IsInternal: false}}
	mock := &mockStaticScraper{
		result: &domain.ScrapeResult{
			URL:      "https://example.com",
			Markdown: "# Example",
			HTML:     `<div><a href="https://iana.org/domains/example">More information...</a></div>`,
			Metadata: map[string]string{"engine": "colly"},
			Links:    wantedLinks,
		},
	}
	// Need to bypass scraper's own extraction: mock returns links directly.
	// Service will pass through since it doesn't re-extract if result already has links.
	svc := scraper.NewService(mock, mock, nil)
	h := NewScrapeHandler(svc)

	body, _ := json.Marshal(ScrapeRequest{URL: "https://example.com", Mode: "static"})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["links"]; !ok {
		t.Error("expected 'links' in response (default true)")
	}
	var links []domain.LinkData
	if err := json.Unmarshal(resp["links"], &links); err != nil {
		t.Fatalf("links unmarshal: %v", err)
	}
	if len(links) != 1 || links[0].URL != wantedLinks[0].URL {
		t.Errorf("links = %+v, want %+v", links, wantedLinks)
	}
}

func TestScrapeHandler_LinksIncludeFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Use a real colly scraper via mock that generates links, and ensure include_links=false suppresses them.
	// We'll test through service directly: handler should set IncludeLinks=false, and scrapers check it.
	// For this test, use a fake HTML with a link; mock ignores opts so we need to verify handler's opt propagation
	// instead via capturing opts.
	type capturingScraper struct {
		opts domain.ScrapeOptions
	}
	cs := &capturingScraper{}
	// Implement Scrape to capture opts and return empty links
	capture := func() domain.Scraper {
		return scraperCapture{fn: func(url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
			cs.opts = opts
			return &domain.ScrapeResult{URL: url, Markdown: "# hi", HTML: "<p>hi</p>", Metadata: map[string]string{}, Links: nil}, nil
		}}
	}
	svc := scraper.NewService(capture(), capture(), nil)
	h := NewScrapeHandler(svc)

	// POST with include_links:false
	body, _ := json.Marshal(ScrapeRequest{URL: "https://example.com", IncludeLinks: boolPtr(false)})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", w.Code, w.Body.String())
	}
	if cs.opts.IncludeLinks == nil || *cs.opts.IncludeLinks != false {
		t.Errorf("IncludeLinks opts = %v, want false", cs.opts.IncludeLinks)
	}
	// Also test query param ?include_links=false on GET
	cs2 := &capturingScraper{}
	svc2 := scraper.NewService(scraperCapture{fn: func(url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
		cs2.opts = opts
		return &domain.ScrapeResult{URL: url, Markdown: "# hi", HTML: "<p>hi</p>"}, nil
	}}, nil, nil)
	h2 := NewScrapeHandler(svc2)
	req2 := httptest.NewRequest("GET", "/scrape?url=https://example.com&include_links=false", nil)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = req2
	h2.Scrape(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d body %s", w2.Code, w2.Body.String())
	}
	if cs2.opts.IncludeLinks == nil || *cs2.opts.IncludeLinks != false {
		t.Errorf("GET IncludeLinks = %v, want false", cs2.opts.IncludeLinks)
	}
}

func boolPtr(b bool) *bool { return &b }

type scraperCapture struct {
	fn func(string, domain.ScrapeOptions) (*domain.ScrapeResult, error)
}

func (s scraperCapture) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	return s.fn(url, opts)
}
