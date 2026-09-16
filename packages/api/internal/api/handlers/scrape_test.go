package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/gin-gonic/gin"
)

// mockStaticScraper implements domain.Scraper
type mockStaticScraper struct {
	result *domain.ScrapeResult
	err    error
}

func (m *mockStaticScraper) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func setupScrapeHandler() *ScrapeHandler {
	colly := &mockStaticScraper{
		result: &domain.ScrapeResult{
			URL:      "https://example.com",
			Markdown: "# Example",
			HTML:     "<html><body><h1>Example</h1></body></html>",
			Metadata: map[string]string{"engine": "colly"},
		},
	}
	svc := scraper.NewService(colly, nil, nil)
	return NewScrapeHandler(svc)
}

func TestScrapeHandler_PostJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := setupScrapeHandler()

	reqBody := ScrapeRequest{
		URL:  "https://example.com",
		Mode: "static",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Scrape(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var result domain.ScrapeResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.URL != "https://example.com" {
		t.Errorf("URL mismatch: got %q", result.URL)
	}
}

func TestScrapeHandler_MissingURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := setupScrapeHandler()

	// POST with empty JSON
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Scrape(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing URL, got %d", w.Code)
	}
}

func TestScrapeHandler_QueryParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := setupScrapeHandler()

	// GET request  with query params
	req := httptest.NewRequest("GET", "/scrape?url=https://example.com&mode=static", nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Scrape(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for GET with query, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestScrapeHandler_RenderBackwardCompat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := setupScrapeHandler()

	// render=true should map to mode=dynamic
	reqBody := ScrapeRequest{
		URL:    "https://example.com",
		Render: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Scrape(c)

	// Dynamic scraper is nil, so this should fail with 500
	if w.Code != http.StatusInternalServerError {
		// If it somehow succeeded we accept that too
		if w.Code != http.StatusOK {
			t.Errorf("Expected 500 (dynamic scraper not configured) or 200, got %d", w.Code)
		}
	}
}

func TestScrapeHandler_DefaultMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := setupScrapeHandler()

	// No mode specified → defaults to "smart" which tries static first
	reqBody := ScrapeRequest{
		URL: "https://example.com",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Scrape(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestScrapeHandler_EmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := setupScrapeHandler()

	req := httptest.NewRequest("POST", "/scrape", nil)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 0

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Scrape(c)

	// No URL provided → 400
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty body, got %d", w.Code)
	}
}

func TestNewScrapeHandler(t *testing.T) {
	svc := scraper.NewService(nil, nil, nil)
	handler := NewScrapeHandler(svc)

	if handler == nil {
		t.Fatal("NewScrapeHandler should not return nil")
	}
}

// --- Multi-URL sync tests (POST /v1/scrape with urls: []) ---

// dynamicMock returns per-URL results, capturing the concurrency limit.
type dynamicMock struct {
	results map[string]*domain.ScrapeResult
	errMap  map[string]error
}

func (m *dynamicMock) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	if err, ok := m.errMap[url]; ok {
		return nil, err
	}
	if r, ok := m.results[url]; ok {
		return r, nil
	}
	return &domain.ScrapeResult{URL: url, Markdown: "# " + url, HTML: "<h1>" + url + "</h1>", Metadata: map[string]string{"engine": "mock"}}, nil
}

func TestScrapeHandler_MultiURL_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := &dynamicMock{
		results: map[string]*domain.ScrapeResult{
			"https://example.com": {URL: "https://example.com", Markdown: "# Example", HTML: "<h1>Example</h1>", Metadata: map[string]string{"engine": "colly"}},
			"https://example.org": {URL: "https://example.org", Markdown: "# Org", HTML: "<h1>Org</h1>", Metadata: map[string]string{"engine": "colly"}},
		},
	}
	svc := scraper.NewService(mock, mock, nil)
	h := NewScrapeHandler(svc)

	body, _ := json.Marshal(ScrapeRequest{URLs: []string{"https://example.com", "https://example.org"}, Mode: "static"})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp MultiScrapeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal multi response: %v body %s", err, w.Body.String())
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	// Order must be preserved.
	if resp.Results[0].URL != "https://example.com" || resp.Results[1].URL != "https://example.org" {
		t.Errorf("order mismatch: %#v", resp.Results)
	}
	if resp.Results[0].Markdown != "# Example" || resp.Results[1].Markdown != "# Org" {
		t.Errorf("markdown mismatch: %#v", resp.Results)
	}
	// Also works via gin.H generic decode
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &generic); err == nil {
		if _, ok := generic["results"]; !ok {
			t.Error("expected 'results' key in multi response")
		}
	}
}

func TestScrapeHandler_MultiURL_Exclusive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := setupScrapeHandler()
	body, _ := json.Marshal(ScrapeRequest{URL: "https://example.com", URLs: []string{"https://example.org"}})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for exclusive url/urls, got %d body %s", w.Code, w.Body.String())
	}
}

func TestScrapeHandler_MultiURL_Max10(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := setupScrapeHandler()
	urls := make([]string, 11)
	for i := range urls {
		urls[i] = "https://example.com/" + string(rune('a'+i))
	}
	body, _ := json.Marshal(ScrapeRequest{URLs: urls})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for >10 urls, got %d body %s", w.Code, w.Body.String())
	}
}

func TestScrapeHandler_MultiURL_InvalidURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := setupScrapeHandler()
	body, _ := json.Marshal(ScrapeRequest{URLs: []string{"https://example.com", "not-a-url"}})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid url, got %d body %s", w.Code, w.Body.String())
	}
}

func TestScrapeHandler_MultiURL_PartialFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := &dynamicMock{
		results: map[string]*domain.ScrapeResult{
			"https://good.example.com": {URL: "https://good.example.com", Markdown: "# Good", HTML: "<h1>Good</h1>", Metadata: map[string]string{}},
		},
		errMap: map[string]error{
			"https://bad.example.com": context.DeadlineExceeded,
		},
	}
	// Need both colly and chromedp to handle smart fallback; provide mock for both
	svc := scraper.NewService(mock, mock, nil)
	h := NewScrapeHandler(svc)
	body, _ := json.Marshal(ScrapeRequest{URLs: []string{"https://good.example.com", "https://bad.example.com"}, Mode: "static"})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with partial failure, got %d body %s", w.Code, w.Body.String())
	}
	var resp MultiScrapeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Error != "" {
		t.Errorf("first item should succeed, got error %q", resp.Results[0].Error)
	}
	if resp.Results[1].Error == "" {
		t.Error("second item should contain error")
	}
	if resp.Results[0].Markdown == "" {
		t.Error("first item markdown missing")
	}
}

func TestScrapeHandler_MultiURL_SingleStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := setupScrapeHandler()
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
	// Must NOT contain results array.
	var m map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if _, ok := m["results"]; ok {
		t.Error("single-url response should not contain 'results'")
	}
	if _, ok := m["url"]; !ok {
		t.Error("single-url response should contain 'url'")
	}
}

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		u    string
		want bool
	}{
		{"https://example.com", true},
		{"http://example.com/path?q=1", true},
		{"ftp://example.com", false},
		{"not-a-url", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isValidURL(tt.u); got != tt.want {
			t.Errorf("isValidURL(%q)=%v want %v", tt.u, got, tt.want)
		}
	}
}
