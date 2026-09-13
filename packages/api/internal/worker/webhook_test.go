package worker

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

func TestSignPayload_HMACSHA256(t *testing.T) {
	secret := "s3cret"
	payload := []byte(`{"status":"completed"}`)
	sig := signPayload(secret, payload)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))
	if sig != want {
		t.Errorf("signature mismatch: got %q want %q", sig, want)
	}
}

func TestDeliver_SignsAndPosts(t *testing.T) {
	var gotBody []byte
	var gotSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get(SignatureHeader)
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := []byte(`{"status":"completed","total":3}`)
	if err := Deliver(context.Background(), srv.URL, "hunter2", payload); err != nil {
		t.Fatalf("deliver failed: %v", err)
	}

	if string(gotBody) != string(payload) {
		t.Errorf("body mismatch: got %q want %q", gotBody, payload)
	}
	wantSig := "sha256=" + signPayload("hunter2", payload)
	if gotSig != wantSig {
		t.Errorf("signature header mismatch: got %q want %q", gotSig, wantSig)
	}
}

func TestDeliver_RetriesTransient(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Deliver(context.Background(), srv.URL, "", []byte(`{}`)); err != nil {
		t.Fatalf("deliver should succeed after retries: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDeliver_NoRetryOn4xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	err := Deliver(context.Background(), srv.URL, "", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for 4xx response")
	}
	if attempts != 1 {
		t.Errorf("4xx must not retry: got %d attempts", attempts)
	}
}

// slowScraper simulates a scrape that takes perPage to complete, providing
// a page tree: the seed links to 4 depth-1 pages.
type slowScraper struct {
	perPage time.Duration
}

func (s *slowScraper) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	select {
	case <-time.After(s.perPage):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	html := "<html><body><h1>" + url + "</h1>"
	if len(url) < 40 { // seed page only
		for i := 1; i <= 4; i++ {
			html += fmt.Sprintf(`<a href="https://slow.example.com/page%d">p%d</a>`, i, i)
		}
	}
	html += "</body></html>"

	return &domain.ScrapeResult{
		URL:      url,
		Markdown: "# " + url,
		HTML:     html,
		Metadata: map[string]string{"engine": "mock"},
	}, nil
}

// TestExecuteCrawl_Parallel verifies depth-1 pages are scraped concurrently:
// 5 pages × 150ms should complete well under the 750ms serial time.
func TestExecuteCrawl_Parallel(t *testing.T) {
	// Disable per-domain politeness so timing reflects parallelism only.
	t.Setenv("CRAWL_DOMAIN_DELAY", "0")

	svc := &slowScraper{perPage: 150 * time.Millisecond}
	handler := NewCrawlTaskHandler(scraper.NewService(svc, nil, nil), newTestLogger())

	start := time.Now()
	result, err := handler.ExecuteCrawl(context.Background(), CrawlPayload{
		URL:      "https://slow.example.com",
		MaxDepth: 1,
		Limit:    5,
	}, "task-parallel")
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}
	elapsed := time.Since(start)

	if result.TotalPages != 5 {
		t.Fatalf("expected 5 pages, got %d", result.TotalPages)
	}
	if elapsed > 600*time.Millisecond {
		t.Errorf("crawl not parallel: took %v", elapsed)
	}
}

// statusScraper fails with the given HTTP status for every URL.
type statusScraper struct {
	status int
	count  sync.Mutex
	hits   int
}

func (s *statusScraper) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	s.count.Lock()
	s.hits++
	s.count.Unlock()
	return nil, &scraper.StatusError{StatusCode: s.status, Err: fmt.Errorf("status %d", s.status)}
}

// TestScrapeWithRetry_4xxNoRetry verifies 4xx errors are never retried.
func TestScrapeWithRetry_4xxNoRetry(t *testing.T) {
	svc := &statusScraper{status: 404}
	_, err := scrapeWithRetry(context.Background(), scraper.NewService(svc, nil, nil), "https://x.example.com", "static", domain.ScrapeOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	svc.count.Lock()
	hits := svc.hits
	svc.count.Unlock()
	if hits != 1 {
		t.Errorf("4xx must not be retried: got %d scrape attempts", hits)
	}
}

// TestScrapeWithRetry_5xxRetries verifies 5xx errors are retried twice.
func TestScrapeWithRetry_5xxRetries(t *testing.T) {
	svc := &statusScraper{status: 500}
	_, err := scrapeWithRetry(context.Background(), scraper.NewService(svc, nil, nil), "https://x.example.com", "static", domain.ScrapeOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	svc.count.Lock()
	hits := svc.hits
	svc.count.Unlock()
	if hits != 3 {
		t.Errorf("expected 3 attempts (1 + 2 retries), got %d", hits)
	}
}

// TestFilterByPatterns verifies include/exclude path filtering.
func TestFilterByPatterns(t *testing.T) {
	links := []string{
		"https://example.com/blog/post-1",
		"https://example.com/blog/post-2",
		"https://example.com/admin/users",
		"https://example.com/about",
	}

	t.Run("exclude wins", func(t *testing.T) {
		got := filterByPatterns(links, nil, []string{"/admin/*"})
		for _, l := range got {
			if l == "https://example.com/admin/users" {
				t.Errorf("admin link should be excluded: %v", got)
			}
		}
		if len(got) != 3 {
			t.Errorf("expected 3 links, got %d: %v", len(got), got)
		}
	})

	t.Run("include restricts", func(t *testing.T) {
		got := filterByPatterns(links, []string{"/blog/*"}, nil)
		if len(got) != 2 {
			t.Errorf("expected 2 blog links, got %d: %v", len(got), got)
		}
	})

	t.Run("empty patterns allow all", func(t *testing.T) {
		got := filterByPatterns(links, nil, nil)
		if len(got) != 4 {
			t.Errorf("expected all links, got %d", len(got))
		}
	})
}
