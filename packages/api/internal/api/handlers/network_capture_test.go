package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/gin-gonic/gin"
)

// TestScrapeHandler_CaptureNetwork covers the wire contract: the request flag
// reaches the service, and the captured responses come back under `network`.
func TestScrapeHandler_CaptureNetwork(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotOpts domain.ScrapeOptions
	capture := scraperCapture{fn: func(url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
		gotOpts = opts
		return &domain.ScrapeResult{
			URL:      url,
			Markdown: "# App",
			HTML:     "<p>App</p>",
			Network: &domain.NetworkCapture{
				Responses: []domain.CapturedResponse{{
					URL: "https://example.com/api/items", Method: "GET", Status: 200,
					ContentType: "application/json", Body: `{"items":[]}`,
				}},
			},
		}, nil
	}}
	svc := scraper.NewService(nil, capture, nil)
	h := NewScrapeHandler(svc)

	body, err := json.Marshal(ScrapeRequest{
		URL:            "https://example.com",
		Mode:           "dynamic",
		CaptureNetwork: true,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.Scrape(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !gotOpts.CaptureNetwork {
		t.Fatal("capture_network was not passed to the service")
	}

	var resp struct {
		Network *domain.NetworkCapture `json:"network"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Network == nil || len(resp.Network.Responses) != 1 {
		t.Fatalf("network = %+v, want one captured response", resp.Network)
	}
	if resp.Network.Responses[0].Body != `{"items":[]}` {
		t.Fatalf("body = %q", resp.Network.Responses[0].Body)
	}
}

// TestScrapeHandler_OmitsNetworkWhenNotRequested keeps the ordinary response
// shape unchanged for callers that never ask for capture.
func TestScrapeHandler_OmitsNetworkWhenNotRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := scraper.NewService(scraperCapture{fn: func(url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
		return &domain.ScrapeResult{URL: url, Markdown: "# App", HTML: "<p>App</p>"}, nil
	}}, nil, nil)
	h := NewScrapeHandler(svc)

	body := []byte(`{"url":"https://example.com","mode":"static"}`)
	req := httptest.NewRequest(http.MethodPost, "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.Scrape(c)

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := raw["network"]; ok {
		t.Fatal("network must be omitted when capture was not requested")
	}
}
