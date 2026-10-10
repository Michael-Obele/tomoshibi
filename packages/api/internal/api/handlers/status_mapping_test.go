package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/gin-gonic/gin"
)

// TestScrapeHandler_UpstreamStatusMapping asserts the handler reports the
// target's own HTTP status instead of collapsing every scrape failure to 500:
// a target 404 stays a 404, any other upstream failure is a 502 (we are the
// gateway that received an invalid response), and only genuinely internal
// errors are 500.
func TestScrapeHandler_UpstreamStatusMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		scrapeErr  error
		wantStatus int
	}{
		{"target 404 stays 404", &scraper.StatusError{StatusCode: http.StatusNotFound, Err: errors.New("missing")}, http.StatusNotFound},
		{"target 403 becomes 502", &scraper.StatusError{StatusCode: http.StatusForbidden, Err: errors.New("blocked")}, http.StatusBadGateway},
		{"target 429 becomes 502", &scraper.StatusError{StatusCode: http.StatusTooManyRequests, Err: errors.New("rate limited")}, http.StatusBadGateway},
		{"target 503 becomes 502", &scraper.StatusError{StatusCode: http.StatusServiceUnavailable, Err: errors.New("unavailable")}, http.StatusBadGateway},
		{"plain error stays 500", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := scraper.NewService(scraperCapture{fn: func(url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
				return nil, tt.scrapeErr
			}}, nil, nil)
			h := NewScrapeHandler(svc)

			body, err := json.Marshal(ScrapeRequest{URL: "https://example.com", Mode: "static"})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			h.Scrape(c)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
