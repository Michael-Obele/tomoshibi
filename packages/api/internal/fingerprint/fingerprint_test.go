package fingerprint

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTransport_BridgesNetHTTP pins the behaviour Colly depends on: the
// tls-client adapter must speak net/http, decompress the body itself, and
// leave redirect handling to the caller.
func TestTransport_BridgesNetHTTP(t *testing.T) {
	// The test server is on loopback, which the SSRF guard refuses by design.
	t.Setenv("SSRF_ALLOW_PRIVATE", "true")

	mux := http.NewServeMux()
	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != ChromeUA {
			t.Errorf("User-Agent = %q, want %q", got, ChromeUA)
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/html")
		gz := gzip.NewWriter(w)
		if _, err := gz.Write([]byte("<html><body><h1>hello</h1></body></html>")); err != nil {
			t.Errorf("write gzip: %v", err)
		}
		if err := gz.Close(); err != nil {
			t.Errorf("close gzip: %v", err)
		}
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/article", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	rt, err := newTransport(true)
	if err != nil {
		t.Fatalf("newTransport: %v", err)
	}
	client := &http.Client{Transport: rt}

	resp, err := client.Get(srv.URL + "/redirect")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "hello") {
		t.Fatalf("body was not decompressed: %q", string(body))
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q after transport decompression, want empty", enc)
	}
}

// TestDefaultHeaders_MatchProfile guards the invariant that makes the whole
// package worthwhile: one identity, not a UA from one browser and a handshake
// from another.
func TestDefaultHeaders_MatchProfile(t *testing.T) {
	headers := DefaultHeaders()
	if got := headers["User-Agent"]; got != ChromeUA {
		t.Fatalf("default User-Agent = %q, want %q", got, ChromeUA)
	}
	if !strings.Contains(ChromeUA, "Chrome/") {
		t.Fatalf("ChromeUA does not look like Chrome: %q", ChromeUA)
	}
}

// TestTransport_PlainHTTP confirms http:// URLs still work: TLS
// impersonation has nothing to do there, and some Chrome profiles refuse the
// scheme outright.
func TestTransport_PlainHTTP(t *testing.T) {
	t.Setenv("SSRF_ALLOW_PRIVATE", "true")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != ChromeUA {
			t.Errorf("User-Agent = %q, want %q", got, ChromeUA)
		}
		fmt.Fprint(w, "<html><body>plain</body></html>")
	}))
	defer srv.Close()

	rt, err := newTransport(true)
	if err != nil {
		t.Fatalf("newTransport: %v", err)
	}
	resp, err := (&http.Client{Transport: rt}).Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "plain") {
		t.Fatalf("body = %q", string(body))
	}
}
