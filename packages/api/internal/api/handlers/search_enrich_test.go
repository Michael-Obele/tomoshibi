package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/search"
	"github.com/gin-gonic/gin"
)

// recordingFetcher stands in for the scraper in handler tests.
type recordingFetcher struct {
	mu    sync.Mutex
	urls  []string
	fail  map[string]bool
	byURL map[string]string
}

func (f *recordingFetcher) Scrape(_ context.Context, url, _ string, _ domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	f.mu.Lock()
	f.urls = append(f.urls, url)
	bad := f.fail[url]
	body := f.byURL[url]
	f.mu.Unlock()
	if bad {
		return nil, errFake
	}
	if body == "" {
		body = "markdown for " + url
	}
	return &domain.ScrapeResult{URL: url, Markdown: body}, nil
}

func (f *recordingFetcher) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.urls...)
}

var errFake = errors.New("fake scrape failure")

func twoResults() search.Service {
	return &MockSearchService{
		SearchFunc: func(_ context.Context, _ search.SearchOptions) ([]search.Result, int, error) {
			return []search.Result{
				{Title: "One", URL: "https://example.com/1", Domain: "example.com"},
				{Title: "Two", URL: "https://example.com/2", Domain: "example.com"},
			}, 2, nil
		},
	}
}

func decodeResults(t *testing.T, w *httptest.ResponseRecorder) SearchResponse {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var resp SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

func TestSearchHandlerScrapeContent(t *testing.T) {
	f := &recordingFetcher{}
	h := NewSearchHandler(twoResults(), WithScraper(f))

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader([]byte(`{"query":"q","scrapeContent":true}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.Search(c)

	resp := decodeResults(t, rec)
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}
	for i, r := range resp.Results {
		if r.Content == "" {
			t.Errorf("result %d has no content", i)
		}
	}
	if got := len(f.calls()); got != 2 {
		t.Errorf("fetched %d urls, want 2", got)
	}
}

func TestSearchHandlerWithoutScrapeContentDoesNotFetch(t *testing.T) {
	f := &recordingFetcher{}
	h := NewSearchHandler(twoResults(), WithScraper(f))

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader([]byte(`{"query":"q"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.Search(c)

	resp := decodeResults(t, rec)
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}
	for i, r := range resp.Results {
		if r.Content != "" {
			t.Errorf("result %d has content without scrapeContent", i)
		}
	}
	if got := len(f.calls()); got != 0 {
		t.Errorf("fetcher called %d times without scrapeContent", got)
	}
}

// TestSearchHandlerScrapeContentWithoutScraperMustNotPanic covers the
// deployment where the handler is built without WithScraper.
func TestSearchHandlerScrapeContentWithoutScraperMustNotPanic(t *testing.T) {
	h := NewSearchHandler(twoResults())

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader([]byte(`{"query":"q","scrapeContent":true}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.Search(c)

	resp := decodeResults(t, rec)
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}
}

func TestSearchHandlerScrapeContentKeepsFailedRows(t *testing.T) {
	f := &recordingFetcher{fail: map[string]bool{"https://example.com/2": true}}
	h := NewSearchHandler(twoResults(), WithScraper(f))

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader([]byte(`{"query":"q","scrapeContent":true}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.Search(c)

	resp := decodeResults(t, rec)
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2 (a failed fetch must not drop the row)", len(resp.Results))
	}
	if resp.Results[0].Content == "" {
		t.Error("successful result should have content")
	}
	if resp.Results[1].Content != "" {
		t.Error("failed result should have empty content")
	}
	if resp.Results[1].Title != "Two" {
		t.Error("failed result lost its SERP title")
	}
}

func TestSearchHandlerScrapeLimit(t *testing.T) {
	f := &recordingFetcher{}
	h := NewSearchHandler(twoResults(), WithScraper(f))

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader([]byte(`{"query":"q","scrapeContent":true,"scrapeLimit":1}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.Search(c)

	resp := decodeResults(t, rec)
	if got := len(f.calls()); got != 1 {
		t.Errorf("fetched %d urls, want 1", got)
	}
	if len(resp.Results) != 2 {
		t.Errorf("results = %d, want 2 (scrapeLimit must not truncate the SERP list)", len(resp.Results))
	}
	if resp.Results[1].Content != "" {
		t.Error("result beyond scrapeLimit should not have content")
	}
}
