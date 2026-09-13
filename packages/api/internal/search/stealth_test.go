package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubFetcher implements BrowserFetcher for tests.
type stubFetcher struct {
	html    string
	err     error
	calls   int
	lastURL string
}

func (s *stubFetcher) FetchHTML(ctx context.Context, url string) (string, error) {
	s.calls++
	s.lastURL = url
	if s.err != nil {
		return "", s.err
	}
	return s.html, nil
}

func TestStealthServiceParsesBraveHTML(t *testing.T) {
	html := `<html><body>
		<div data-type='web'><a href="https://go.dev/">The Go Programming Language</a></div>
		<div data-type='web'><a href="https://example.org/">Example Domain</a></div>
	</body></html>`
	fetcher := &stubFetcher{html: html}
	svc := NewStealthService(fetcher, "https://search.brave.com")
	results, total, err := svc.Search(context.Background(), SearchOptions{Query: "golang", Limit: 2})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if total != 2 {
		t.Errorf("total=%d want 2", total)
	}
	if len(results) != 2 {
		t.Fatalf("len=%d want 2", len(results))
	}
	if results[0].URL != "https://go.dev/" {
		t.Errorf("URL=%q want https://go.dev/", results[0].URL)
	}
	if results[0].Domain != "go.dev" {
		t.Errorf("Domain=%q want go.dev", results[0].Domain)
	}
}

func TestStealthServiceRespectsContextCancellation(t *testing.T) {
	fetcher := &stubFetcher{err: context.Canceled}
	svc := NewStealthService(fetcher, "https://search.brave.com")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := svc.Search(ctx, SearchOptions{Query: "q"})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestStealthServiceRotatesUA(t *testing.T) {
	t.Setenv("SSRF_ALLOW_PRIVATE", "true")
	var uas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uas = append(uas, r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte(`<html><body><a href="https://a.dev/">Title A long enough</a></body></html>`))
	}))
	defer srv.Close()
	svc := NewStealthService(nil, srv.URL)
	svc.client = srv.Client()
	_, _, _ = svc.Search(context.Background(), SearchOptions{Query: "q1"})
	_, _, _ = svc.Search(context.Background(), SearchOptions{Query: "q2"})
	if len(uas) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(uas))
	}
	if uas[0] == "" || uas[1] == "" {
		t.Error("expected non-empty User-Agent")
	}
}

func TestStealthServiceHonorsLimit(t *testing.T) {
	html := `<html><body>
		<a href="https://a.dev/">Title A long enough</a>
		<a href="https://b.dev/">Title B long enough</a>
		<a href="https://c.dev/">Title C long enough</a>
	</body></html>`
	fetcher := &stubFetcher{html: html}
	svc := NewStealthService(fetcher, "https://search.brave.com")
	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "x", Limit: 2})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("len=%d want 2", len(results))
	}
}

func TestStealthServiceEmptyHTMLReturnsEmpty(t *testing.T) {
	fetcher := &stubFetcher{html: `<html><body></body></html>`}
	svc := NewStealthService(fetcher, "https://search.brave.com")
	results, _, err := svc.Search(context.Background(), SearchOptions{Query: "q"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty HTML, got %d", len(results))
	}
}

func TestStealthServiceValidatesCategory(t *testing.T) {
	fetcher := &stubFetcher{html: `<html></html>`}
	svc := NewStealthService(fetcher, "https://search.brave.com")
	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "q", Category: "invalid"})
	if err == nil || !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("expected invalid category error, got %v", err)
	}
}
