package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testRouter(handlers ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.GET("/v1/test", handlers...)
	return r
}

func doGet(r *gin.Engine, path string, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if apiKey != "" {
		req.Header.Set(APIKeyHeader, apiKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAPIKeyAuth_OpenWhenNoKeys(t *testing.T) {
	r := testRouter(APIKeyAuth(nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	if w := doGet(r, "/v1/test", ""); w.Code != http.StatusOK {
		t.Errorf("open mode should pass: got %d", w.Code)
	}
}

func TestAPIKeyAuth_RejectsMissingKey(t *testing.T) {
	r := testRouter(APIKeyAuth([]string{"sekret"}), func(c *gin.Context) { c.Status(http.StatusOK) })
	if w := doGet(r, "/v1/test", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("missing key should 401: got %d", w.Code)
	}
}

func TestAPIKeyAuth_RejectsBadKey(t *testing.T) {
	r := testRouter(APIKeyAuth([]string{"sekret"}), func(c *gin.Context) { c.Status(http.StatusOK) })
	if w := doGet(r, "/v1/test", "wrong"); w.Code != http.StatusUnauthorized {
		t.Errorf("bad key should 401: got %d", w.Code)
	}
}

func TestAPIKeyAuth_AcceptsValidKey(t *testing.T) {
	r := testRouter(APIKeyAuth([]string{"sekret"}), func(c *gin.Context) { c.Status(http.StatusOK) })
	if w := doGet(r, "/v1/test", "sekret"); w.Code != http.StatusOK {
		t.Errorf("valid key should pass: got %d", w.Code)
	}
}

func TestRateLimit_ExceedsRPM(t *testing.T) {
	r := testRouter(RateLimit(2, nil), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 1; i <= 2; i++ {
		if w := doGet(r, "/v1/test", "k"); w.Code != http.StatusOK {
			t.Errorf("request %d should pass: got %d", i, w.Code)
		}
	}
	if w := doGet(r, "/v1/test", "k"); w.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request should 429: got %d", w.Code)
	}
	// Different client is unaffected.
	if w := doGet(r, "/v1/test", "other"); w.Code != http.StatusOK {
		t.Errorf("different client should pass: got %d", w.Code)
	}
}

func TestRateLimit_DisabledWhenZero(t *testing.T) {
	r := testRouter(RateLimit(0, nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	for i := 0; i < 5; i++ {
		if w := doGet(r, "/v1/test", ""); w.Code != http.StatusOK {
			t.Fatalf("unlimited mode should pass: got %d", w.Code)
		}
	}
}
