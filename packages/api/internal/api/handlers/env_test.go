package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/search/envctx"
	"github.com/gin-gonic/gin"
)

// TestEnvHandlerListsAllowedNames pins the MCP's discovery contract:
// GET /v1/env returns {header, env:[names]} so a client knows which of its
// own env vars it may forward. Values must never appear in the response.
func TestEnvHandlerListsAllowedNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/env", Env)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/env", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body struct {
		Header string   `json:"header"`
		Env    []string `json:"env"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Header != envctx.Header {
		t.Errorf("header = %q, want %q", body.Header, envctx.Header)
	}

	want := envctx.Allowed()
	if len(body.Env) != len(want) {
		t.Fatalf("env = %v, want %v", body.Env, want)
	}
	for i, name := range want {
		if body.Env[i] != name {
			t.Errorf("env[%d] = %q, want %q", i, body.Env[i], name)
		}
	}
}
