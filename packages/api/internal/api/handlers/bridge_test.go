package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/bridge"
	"github.com/gin-gonic/gin"
)

func newBridgeTestServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	state, err := bridge.NewState(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	h := NewBridgeHandler(bridge.NewServer(state, nil))
	r := gin.New()
	r.GET("/v1/bridge", h.Status)
	r.POST("/v1/bridge/allow", h.Allow)
	r.POST("/v1/bridge/deny", h.Deny)
	r.POST("/v1/bridge/unpair", h.Unpair)
	return r
}

func doJSON(t *testing.T, r http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: bad JSON %q: %v", method, path, w.Body.String(), err)
		}
	}
	return w, out
}

func TestBridgeStatusReportsDefaults(t *testing.T) {
	r := newBridgeTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/v1/bridge", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["enabled"] != true {
		t.Errorf("enabled = %v, want true", body["enabled"])
	}
	allowed, _ := body["allowed_origins"].([]any)
	if len(allowed) != len(bridge.DefaultAllowedOrigins()) {
		t.Errorf("allowed_origins = %v, want the %d defaults", allowed, len(bridge.DefaultAllowedOrigins()))
	}
	if _, ok := body["pairing_code"]; ok {
		t.Errorf("pairing_code present without ?code=1")
	}
}

func TestBridgeStatusIncludesCodeOnRequest(t *testing.T) {
	r := newBridgeTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/v1/bridge?code=1", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	code, _ := body["pairing_code"].(string)
	if len(code) != 6 {
		t.Errorf("pairing_code = %q, want 6 digits", code)
	}
}

func TestBridgeAllowThenDeny(t *testing.T) {
	r := newBridgeTestServer(t)

	_, allow := doJSON(t, r, http.MethodPost, "/v1/bridge/allow", `{"hosts":["Example.COM/docs","*.corp.test"]}`)
	added, _ := allow["added"].([]any)
	if len(added) != 2 {
		t.Fatalf("added = %v, want 2 entries", added)
	}
	if added[0] != "example.com" {
		t.Errorf("added[0] = %v, want the normalized example.com", added[0])
	}

	_, deny := doJSON(t, r, http.MethodPost, "/v1/bridge/deny", `{"hosts":["example.com"]}`)
	removed, _ := deny["removed"].([]any)
	if len(removed) != 1 || removed[0] != "example.com" {
		t.Errorf("removed = %v, want [example.com]", removed)
	}
}

func TestBridgeAllowRejectsNonHosts(t *testing.T) {
	r := newBridgeTestServer(t)
	w, body := doJSON(t, r, http.MethodPost, "/v1/bridge/allow", `{"hosts":[""]}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an empty hosts list", w.Code)
	}
	if body["error"] == nil {
		t.Errorf("expected an error message")
	}
}

func TestBridgeUnpairEmptyBodyRevokesAll(t *testing.T) {
	r := newBridgeTestServer(t)
	w, body := doJSON(t, r, http.MethodPost, "/v1/bridge/unpair", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["removed"] != float64(0) {
		t.Errorf("removed = %v, want 0 with nothing paired", body["removed"])
	}
}

func TestBridgeStatusWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewBridgeHandler(nil)
	r := gin.New()
	r.GET("/v1/bridge", h.Status)

	_, body := doJSON(t, r, http.MethodGet, "/v1/bridge", "")
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false", body["enabled"])
	}
	if body["hint"] == nil {
		t.Errorf("expected a hint explaining the bridge is off")
	}
}
