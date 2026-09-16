package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/config"
	"github.com/gin-gonic/gin"
)

func TestRouter_HealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{Server: config.ServerConfig{Mode: "test"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// nil handlers are fine for the health routes; crawl routes degrade to 503.
	r := NewRouter(cfg, logger, nil, nil, nil, nil)

	for _, path := range []string{"/health", "/v1/ping"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", path, w.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if body["status"] != "ok" {
				t.Errorf("GET %s status = %q, want \"ok\"", path, body["status"])
			}
		})
	}
}
