package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/chromedp/cdproto/network"
)

func TestCapturableMIME(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/problem+json", true},
		{"application/x-ndjson", true},
		{"application/xml", true},
		{"text/xml", true},
		{"text/plain", true},
		{"text/csv", true},
		{"application/x-www-form-urlencoded", true},
		{"text/html", false},
		{"application/javascript", false},
		{"image/png", false},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			if got := capturableMIME(tt.mime); got != tt.want {
				t.Fatalf("capturableMIME(%q) = %v, want %v", tt.mime, got, tt.want)
			}
		})
	}
}

func TestCapturableType(t *testing.T) {
	tests := []struct {
		name   string
		typ    network.ResourceType
		mime   string
		status int
		want   bool
	}{
		{"xhr json", network.ResourceTypeXHR, "application/json", 200, true},
		{"fetch json", network.ResourceTypeFetch, "application/json", 200, true},
		{"document json is not captured", network.ResourceTypeDocument, "application/json", 200, false},
		{"image is not captured", network.ResourceTypeImage, "image/png", 200, false},
		{"script is not captured", network.ResourceTypeScript, "text/plain", 200, false},
		{"204 has no body", network.ResourceTypeXHR, "application/json", 204, false},
		{"304 has no body", network.ResourceTypeFetch, "application/json", 304, false},
		{"html fragment is not data", network.ResourceTypeFetch, "text/html", 200, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := capturableType(tt.typ, tt.mime, tt.status); got != tt.want {
				t.Fatalf("capturableType = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedactCapturedURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "token is redacted",
			in:   "https://api.example.com/items?token=abc123&page=2",
			want: "https://api.example.com/items?page=2&token=%5Bredacted%5D",
		},
		{
			name: "harmless query is untouched",
			in:   "https://api.example.com/items?page=2&sort=asc",
			want: "https://api.example.com/items?page=2&sort=asc",
		},
		{
			name: "api key is redacted",
			in:   "https://api.example.com/x?API_KEY=secret",
			want: "https://api.example.com/x?API_KEY=%5Bredacted%5D",
		},
		{
			name: "invalid URL is returned as-is",
			in:   "://not a url",
			want: "://not a url",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactCapturedURL(tt.in); got != tt.want {
				t.Fatalf("redactCapturedURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCollectorRecord_DedupsIdenticalBodies(t *testing.T) {
	c := &networkCollector{seen: make(map[string]int)}
	entry := capturedEntry{
		url: "https://api.example.com/items", method: "GET", status: 200,
		contentType: "application/json", resourceType: "XHR",
	}
	c.record(entry, `{"items":[]}`)
	c.record(entry, `{"items":[]}`)
	c.record(entry, `{"items":[1]}`)

	if len(c.responses) != 2 {
		t.Fatalf("responses = %d, want 2 (duplicate collapsed)", len(c.responses))
	}
	if c.responses[0].Count != 2 {
		t.Fatalf("first response count = %d, want 2", c.responses[0].Count)
	}
	if c.total != len(`{"items":[]}`)+len(`{"items":[1]}`) {
		t.Fatalf("total = %d, want the sum of both bodies", c.total)
	}
}

func TestCollectorRecord_OmitsBodyWhenBudgetExhausted(t *testing.T) {
	c := &networkCollector{seen: make(map[string]int), total: captureMaxTotalBytes}
	c.record(capturedEntry{
		url: "https://api.example.com/items", method: "GET", status: 200,
		contentType: "application/json",
	}, `{"items":[]}`)

	if len(c.responses) != 1 {
		t.Fatalf("responses = %d, want 1", len(c.responses))
	}
	got := c.responses[0]
	if got.Body != "" {
		t.Fatalf("body = %q, want omitted", got.Body)
	}
	if !got.Truncated || !c.truncated {
		t.Fatal("budget exhaustion must mark the response and the capture as truncated")
	}
	if got.BodyError != "capture budget exhausted" {
		t.Fatalf("body_error = %q", got.BodyError)
	}
}

func TestCollectorRecord_KeepsMetadataWhenBodyIsMissing(t *testing.T) {
	c := &networkCollector{seen: make(map[string]int)}
	c.record(capturedEntry{
		url: "https://api.example.com/items", method: "GET", status: 200,
		contentType: "application/json", sizeBytes: 4096,
		bodyError: "non-text body omitted",
	}, "")

	got := c.responses[0]
	if got.SizeBytes != 4096 {
		t.Fatalf("size_bytes = %d, want the encoded length", got.SizeBytes)
	}
	if got.BodyError != "non-text body omitted" {
		t.Fatalf("body_error = %q", got.BodyError)
	}
	if got.Truncated {
		t.Fatal("a non-text omission is not a truncation")
	}
}

// chromeAvailable reports whether a Chrome/Chromium binary is reachable. The
// integration test below skips rather than failing on a machine without one.
func chromeAvailable() bool {
	if bin := os.Getenv("CHROME_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return true
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// TestChromedpCaptureNetwork_XHR is the end-to-end check: a page fetches JSON
// after load, and the collector returns it while ignoring the document and the
// page's other resources.
func TestChromedpCaptureNetwork_XHR(t *testing.T) {
	if !chromeAvailable() {
		t.Skip("no Chrome/Chromium binary available")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><h1>App</h1><div id="out">loading</div>
<script>
fetch('/api/items').then(r => r.json()).then(d => {
  document.getElementById('out').textContent = d.items.length;
});
</script></body></html>`)
	})
	mux.HandleFunc("/api/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[{"id":1},{"id":2}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewChromedpScraperWithLimit(10)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	result, err := s.Scrape(ctx, srv.URL, domain.ScrapeOptions{CaptureNetwork: true})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Network == nil {
		t.Fatal("Network capture was requested but returned nil")
	}

	var found *domain.CapturedResponse
	for i := range result.Network.Responses {
		if strings.HasSuffix(result.Network.Responses[i].URL, "/api/items") {
			found = &result.Network.Responses[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("XHR response not captured: %+v", result.Network.Responses)
	}
	if found.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", found.Status)
	}
	if !strings.Contains(found.Body, `"items"`) {
		t.Fatalf("body = %q, want the JSON payload", found.Body)
	}
	if found.ContentType != "application/json" {
		t.Fatalf("content_type = %q", found.ContentType)
	}
	if found.ResourceType != "XHR" && found.ResourceType != "Fetch" {
		t.Fatalf("resource_type = %q, want XHR or Fetch", found.ResourceType)
	}

	for _, r := range result.Network.Responses {
		if strings.HasSuffix(r.URL, "/") || r.ResourceType == "Document" {
			t.Fatalf("the document itself was captured: %+v", r)
		}
	}
}
