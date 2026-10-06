package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/search/envctx"
	"github.com/gin-gonic/gin"
)

// serveWithEnv runs one GET through RequestEnv and reports the status plus
// what the handler resolved for BRAVE_SEARCH_API_KEY. A nil headerValue
// sends no X-Tomoshi-Env header at all.
func serveWithEnv(t *testing.T, headerValue *string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestEnv(nil))
	resolved := ""
	r.GET("/", func(c *gin.Context) {
		resolved = envctx.Get(c.Request.Context(), "BRAVE_SEARCH_API_KEY")
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if headerValue != nil {
		req.Header.Set(envctx.Header, *headerValue)
	}
	r.ServeHTTP(w, req)
	return w.Code, resolved
}

func TestRequestEnvOverridesProcessEnv(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "from-process")

	hdr := `{"BRAVE_SEARCH_API_KEY":"from-header"}`
	code, got := serveWithEnv(t, &hdr)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got != "from-header" {
		t.Errorf("resolved = %q, want the header override", got)
	}

	// No header: behaviour is unchanged, the process env wins.
	code, got = serveWithEnv(t, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got != "from-process" {
		t.Errorf("resolved = %q, want the process env", got)
	}
}

func TestRequestEnvIgnoresMalformedHeader(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "from-process")

	for _, hdr := range []string{
		"not-json",
		`{"BRAVE_SEARCH_API_KEY":42}`,
		`["BRAVE_SEARCH_API_KEY"]`,
	} {
		h := hdr
		code, got := serveWithEnv(t, &h)
		if code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (never a 4xx)", h, code)
		}
		if got != "from-process" {
			t.Errorf("%s: resolved = %q, want process env (header ignored)", h, got)
		}
	}
}

func TestRequestEnvDropsUnknownNames(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "")

	// Only a non-allowlisted name is supplied, so no override attaches and
	// the request still succeeds.
	hdr := `{"REDIS_URL":"redis://evil"}`
	code, got := serveWithEnv(t, &hdr)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got != "" {
		t.Errorf("resolved = %q, want empty (non-allowlisted name dropped)", got)
	}
}
