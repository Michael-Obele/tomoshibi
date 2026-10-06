package engines

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/search/envctx"
)

func testSpec(path string) *Spec {
	return &Spec{
		Name:       "test",
		Categories: []string{"general"},
		TimeoutMS:  2000,
		Request:    RequestSpec{URL: path},
		Parse: ParseSpec{
			Type: "css",
			Item: "li.r",
			Fields: map[string]FieldSpec{
				"title":   {Sel: "a", Attr: "text"},
				"url":     {Sel: "a", Attr: "href"},
				"content": {Sel: "p", Attr: "text"},
			},
		},
	}
}

func TestEngineHTTPFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query().Get("q"); q != "web scraping" {
			t.Errorf("query not rendered: %q", q)
		}
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("User-Agent must be set")
		}
		_, _ = w.Write([]byte(`<ul><li class="r"><a href="https://x.dev/1">T1</a><p>C1</p></li></ul>`))
	})
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/ratelimited", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	mux.HandleFunc("/captcha", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>please solve this anomaly-modal to continue</html>`))
	})
	mux.HandleFunc("/teapot", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pool := newTransportPool(nil)

	t.Run("success", func(t *testing.T) {
		spec := testSpec(srv.URL + "/ok?q={{query}}")
		res, err := newEngineHTTP(spec, pool).fetch(context.Background(), Query{Q: "web scraping"})
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if len(res) != 1 || res[0].Title != "T1" || res[0].URL != "https://x.dev/1" {
			t.Fatalf("results = %+v", res)
		}
		if res[0].Engine != "test" {
			t.Errorf("engine name not stamped: %q", res[0].Engine)
		}
	})

	t.Run("403/429 are blocked", func(t *testing.T) {
		for _, path := range []string{"/forbidden", "/ratelimited"} {
			spec := testSpec(srv.URL + path)
			_, err := newEngineHTTP(spec, pool).fetch(context.Background(), Query{Q: "x"})
			if !errors.Is(err, ErrBlocked) {
				t.Errorf("%s → %v, want ErrBlocked", path, err)
			}
		}
	})

	t.Run("marker in 200 body is blocked", func(t *testing.T) {
		spec := testSpec(srv.URL + "/captcha")
		spec.BlockMarkers = []string{"anomaly-modal"}
		_, err := newEngineHTTP(spec, pool).fetch(context.Background(), Query{Q: "x"})
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("captcha page → %v, want ErrBlocked", err)
		}
	})

	t.Run("other statuses are plain errors", func(t *testing.T) {
		spec := testSpec(srv.URL + "/teapot")
		_, err := newEngineHTTP(spec, pool).fetch(context.Background(), Query{Q: "x"})
		if err == nil || errors.Is(err, ErrBlocked) {
			t.Errorf("418 → %v, want plain error", err)
		}
	})

	t.Run("missing requires_env skips cleanly", func(t *testing.T) {
		spec := testSpec(srv.URL + "/ok")
		spec.RequiresEnv = "TOMOSHIBI_TEST_MISSING_KEY_XYZ"
		_, err := newEngineHTTP(spec, pool).fetch(context.Background(), Query{Q: "x"})
		if !errors.Is(err, ErrNotConfigured) {
			t.Errorf("→ %v, want ErrNotConfigured", err)
		}
	})

	t.Run("requires_env satisfied by per-request override", func(t *testing.T) {
		spec := testSpec(srv.URL + "/ok?q={{query}}")
		spec.RequiresEnv = "TOMOSHIBI_TEST_OVERRIDDEN_KEY_XYZ"
		ctx := envctx.With(context.Background(), envctx.Overrides{
			"TOMOSHIBI_TEST_OVERRIDDEN_KEY_XYZ": "supplied-by-client",
		})
		if _, err := newEngineHTTP(spec, pool).fetch(ctx, Query{Q: "web scraping"}); err != nil {
			t.Fatalf("fetch with override: %v", err)
		}
	})

	t.Run("header ${ENV} resolves from per-request override", func(t *testing.T) {
		srvKeyed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Test-Key"); got != "supplied-by-client" {
				t.Errorf("header = %q, want override value", got)
			}
			_, _ = w.Write([]byte(`<ul><li class="r"><a href="https://x.dev/1">T1</a><p>C1</p></li></ul>`))
		}))
		defer srvKeyed.Close()
		spec := testSpec(srvKeyed.URL + "/ok?q={{query}}")
		spec.Request.Headers = map[string]string{"X-Test-Key": "${TOMOSHIBI_TEST_HDR_KEY_XYZ}"}
		ctx := envctx.With(context.Background(), envctx.Overrides{
			"TOMOSHIBI_TEST_HDR_KEY_XYZ": "supplied-by-client",
		})
		if _, err := newEngineHTTP(spec, pool).fetch(ctx, Query{Q: "x"}); err != nil {
			t.Fatalf("fetch: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(300 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		}))
		defer slow.Close()
		spec := testSpec(slow.URL)
		spec.TimeoutMS = 50
		_, err := newEngineHTTP(spec, pool).fetch(context.Background(), Query{Q: "x"})
		if err == nil {
			t.Fatal("expected timeout error")
		}
	})
}

func TestNewEnginesRoster(t *testing.T) {
	reg, err := LoadRegistry("")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	engs := NewEngines(reg, NewProxyPool(ProxyOptions{}))
	if len(engs) == 0 {
		t.Fatal("no engines built")
	}
	seen := map[string]bool{}
	for _, e := range engs {
		if seen[e.Name()] {
			t.Errorf("duplicate engine %q", e.Name())
		}
		seen[e.Name()] = true
		if len(e.Categories()) == 0 {
			t.Errorf("%s has no categories", e.Name())
		}
	}
	// Disabled entries (google, startpage) must not be materialized.
	if seen["google"] || seen["startpage"] {
		t.Error("disabled engines must not be built")
	}
}
