package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/gin-gonic/gin"
)

func TestScrapeHandler_JSExtractionParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// `capturingScraper` in links_test.go is function-local, so capture into a
	// plain variable here instead of redefining that type.
	var captured domain.ScrapeOptions
	capture := scraperCapture{fn: func(url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
		captured = opts
		return &domain.ScrapeResult{
			URL:           url,
			Markdown:      "# hi",
			HTML:          "<p>hi</p>",
			Metadata:      map[string]string{},
			Evaluations:   []domain.EvaluationResult{{Type: "evaluate", Result: 42}},
			ExtractedData: &domain.ExtractedData{Sources: []domain.DataSource{{Source: "json_ld", Data: "x"}}},
		}, nil
	}}
	// Page actions require a real browser, and the service rejects actions in
	// static mode: drive the dynamic path with the capture as the engine.
	svc := scraper.NewService(capture, capture, nil)
	h := NewScrapeHandler(svc)

	body := []byte(`{
		"url": "https://example.com",
		"mode": "dynamic",
		"actions": [
			{"type": "evaluate", "script": "() => 1"},
			{"type": "wait_for_function", "script": "() => true", "ms": 1000}
		],
		"auto_extract": false,
		"screenshot_opts": {"scale": "css"}
	}`)
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", w.Code, w.Body.String())
	}
	got := captured.Actions
	if len(got) != 2 {
		t.Fatalf("actions = %+v, want 2", got)
	}
	if got[0].Script != "() => 1" || got[0].Type != "evaluate" {
		t.Errorf("action 0 = %+v", got[0])
	}
	if got[1].Script != "() => true" || got[1].Ms != 1000 {
		t.Errorf("action 1 = %+v", got[1])
	}
	if captured.AutoExtract == nil || *captured.AutoExtract != false {
		t.Errorf("AutoExtract = %v, want false", captured.AutoExtract)
	}
	if captured.ScreenshotOpts == nil || captured.ScreenshotOpts.Scale != "css" {
		t.Errorf("ScreenshotOpts = %+v, want scale css", captured.ScreenshotOpts)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"evaluations"`)) {
		t.Errorf("response missing evaluations: %s", w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"extracted_data"`)) {
		t.Errorf("response missing extracted_data: %s", w.Body.String())
	}
}

func TestScrapeHandler_TooManyActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := scraper.NewService(scraperCapture{fn: func(url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
		return &domain.ScrapeResult{URL: url, Markdown: "# hi", HTML: "<p>hi</p>"}, nil
	}}, nil, nil)
	h := NewScrapeHandler(svc)

	actions := make([]ActionReq, maxActionsPerRequest+1)
	for i := range actions {
		actions[i] = ActionReq{Type: "evaluate", Script: "() => " + strconv.Itoa(i)}
	}
	body, _ := json.Marshal(ScrapeRequest{URL: "https://example.com", Actions: actions})
	req := httptest.NewRequest("POST", "/scrape", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Scrape(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body %s", w.Code, w.Body.String())
	}
}
