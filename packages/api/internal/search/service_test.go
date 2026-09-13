package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// newTestService points a BraveService at a stub upstream and removes the
// production rate limit (1 request per 1.1s), which would otherwise add over a
// second to every test that issues more than one search. The endpoint field
// exists for exactly this.
func newTestService(t *testing.T, handler http.HandlerFunc) *BraveService {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	s := NewBraveService("test-api-key")
	s.endpoint = srv.URL
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s
}

// respondWith returns a handler serving n synthetic web results, and records
// the query string of the last request it saw.
func respondWith(n int, gotQuery *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if gotQuery != nil {
			*gotQuery = r.URL.RawQuery
		}
		var items []string
		for i := 0; i < n; i++ {
			items = append(items, fmt.Sprintf(
				`{"title":"Title %d","url":"https://example%d.com/page","description":"Desc %d"}`,
				i, i, i))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"web":{"results":[%s]}}`, strings.Join(items, ","))
	}
}

// TestSearchMapsUpstreamResults covers the response-to-Result conversion:
// field copying, the synthesized ID, domain extraction and the positional
// relevance score.
func TestSearchMapsUpstreamResults(t *testing.T) {
	svc := newTestService(t, respondWith(3, nil))

	results, _, err := svc.Search(context.Background(), SearchOptions{
		Query:  "golang",
		Offset: 10,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search returned an error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	first := results[0]
	if first.Title != "Title 0" {
		t.Errorf("Title = %q, want %q", first.Title, "Title 0")
	}
	if first.URL != "https://example0.com/page" {
		t.Errorf("URL = %q, want %q", first.URL, "https://example0.com/page")
	}
	if first.Description != "Desc 0" {
		t.Errorf("Description = %q, want %q", first.Description, "Desc 0")
	}
	if first.Domain != "example0.com" {
		t.Errorf("Domain = %q, want %q", first.Domain, "example0.com")
	}

	// ID is query + absolute position, so it stays unique across pages.
	if want := "golang_10"; first.ID != want {
		t.Errorf("ID = %q, want %q", first.ID, want)
	}
	if want := "golang_12"; results[2].ID != want {
		t.Errorf("third ID = %q, want %q", results[2].ID, want)
	}

	// Relevance decays 0.05 per position.
	for i, r := range results {
		want := 1.0 - float64(i)*0.05
		if diff := r.Relevance - want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("result %d Relevance = %v, want %v", i, r.Relevance, want)
		}
	}
}

// TestSearchSendsAuthAndQuery asserts the request Cinder actually builds: the
// subscription header carries the key, and the query is passed through.
func TestSearchSendsAuthAndQuery(t *testing.T) {
	var gotToken, gotAccept, gotQuery string
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Subscription-Token")
		gotAccept = r.Header.Get("Accept")
		gotQuery = r.URL.Query().Get("q")
		respondWith(1, nil)(w, r)
	})

	if _, _, err := svc.Search(context.Background(), SearchOptions{Query: "go generics"}); err != nil {
		t.Fatalf("Search returned an error: %v", err)
	}

	if gotToken != "test-api-key" {
		t.Errorf("X-Subscription-Token = %q, want %q", gotToken, "test-api-key")
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if gotQuery != "go generics" {
		t.Errorf("q = %q, want %q", gotQuery, "go generics")
	}
}

// TestSearchQueryParams covers the option-to-query-parameter translation:
// the count/offset pagination pair and the two independent paths that can set
// freshness.
func TestSearchQueryParams(t *testing.T) {
	day := 1
	week := 7
	month := 30
	unmapped := 90

	tests := []struct {
		name string
		opts SearchOptions
		want map[string]string // param -> expected value; "" means absent
	}{
		{
			name: "zero limit defaults to 10 and omits offset",
			opts: SearchOptions{Query: "q"},
			want: map[string]string{"count": "10", "offset": ""},
		},
		{
			name: "limit above 100 is capped",
			opts: SearchOptions{Query: "q", Limit: 500},
			want: map[string]string{"count": "100"},
		},
		{
			name: "offset is forwarded",
			opts: SearchOptions{Query: "q", Limit: 20, Offset: 40},
			want: map[string]string{"count": "20", "offset": "40"},
		},
		{
			name: "fast mode asks for day freshness",
			opts: SearchOptions{Query: "q", Mode: "fast"},
			want: map[string]string{"freshness": "day"},
		},
		{
			name: "non-fast mode sets no freshness",
			opts: SearchOptions{Query: "q", Mode: "thorough"},
			want: map[string]string{"freshness": ""},
		},
		{
			name: "max age 1 maps to day",
			opts: SearchOptions{Query: "q", MaxAge: &day},
			want: map[string]string{"freshness": "day"},
		},
		{
			name: "max age 7 maps to week",
			opts: SearchOptions{Query: "q", MaxAge: &week},
			want: map[string]string{"freshness": "week"},
		},
		{
			name: "max age 30 maps to month",
			opts: SearchOptions{Query: "q", MaxAge: &month},
			want: map[string]string{"freshness": "month"},
		},
		{
			// Only 1, 7 and 30 are mapped; anything else is silently dropped
			// rather than passed through as an invalid freshness value.
			name: "unmapped max age sets no freshness",
			opts: SearchOptions{Query: "q", MaxAge: &unmapped},
			want: map[string]string{"freshness": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw string
			svc := newTestService(t, respondWith(1, &raw))

			if _, _, err := svc.Search(context.Background(), tt.opts); err != nil {
				t.Fatalf("Search returned an error: %v", err)
			}

			q, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatalf("could not parse recorded query %q: %v", raw, err)
			}
			for param, want := range tt.want {
				if got := q.Get(param); got != want {
					t.Errorf("%s = %q, want %q (full query: %s)", param, got, want, raw)
				}
			}
		})
	}
}

// TestSearchTotalCountEstimate covers the pagination signal the handler relies
// on to decide whether a next page exists.
func TestSearchTotalCountEstimate(t *testing.T) {
	tests := []struct {
		name    string
		serve   int
		opts    SearchOptions
		want    int
		comment string
	}{
		{
			// A short page means the result set is exhausted, so the total is
			// exact and pagination should stop.
			name:  "partial page reports exact total",
			serve: 3,
			opts:  SearchOptions{Query: "q", Limit: 10, Offset: 0},
			want:  3,
		},
		{
			name:  "partial page accounts for the offset",
			serve: 3,
			opts:  SearchOptions{Query: "q", Limit: 10, Offset: 20},
			want:  23,
		},
		{
			// A full page means there may be more, so the estimate is padded
			// to keep the caller paginating.
			name:  "full page pads the estimate",
			serve: 10,
			opts:  SearchOptions{Query: "q", Limit: 10, Offset: 0},
			want:  110,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, respondWith(tt.serve, nil))

			_, total, err := svc.Search(context.Background(), tt.opts)
			if err != nil {
				t.Fatalf("Search returned an error: %v", err)
			}
			if total != tt.want {
				t.Errorf("total = %d, want %d", total, tt.want)
			}
		})
	}
}

// TestSearchUpstreamFailures covers every way the request can fail after it
// leaves Cinder. None of these should panic or return partial results.
func TestSearchUpstreamFailures(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name: "rate limited",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantErr: "status 429",
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: "status 500",
		},
		{
			name: "unauthorized",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: "status 401",
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"web":{"results":[`)
			},
			wantErr: "failed to decode response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, tt.handler)

			results, total, err := svc.Search(context.Background(), SearchOptions{Query: "q"})
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantErr)
			}
			if results != nil {
				t.Errorf("expected no results alongside the error, got %d", len(results))
			}
			if total != 0 {
				t.Errorf("expected a zero total alongside the error, got %d", total)
			}
		})
	}
}

// TestSearchEmptyResults asserts an empty upstream page is a valid answer, not
// an error — the handler distinguishes "found nothing" from "failed".
func TestSearchEmptyResults(t *testing.T) {
	svc := newTestService(t, respondWith(0, nil))

	results, total, err := svc.Search(context.Background(), SearchOptions{Query: "q", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned an error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}

// TestSearchRespectsContextCancellation asserts a cancelled request does not
// hang waiting on an upstream that never answers.
func TestSearchRespectsContextCancellation(t *testing.T) {
	blocked := make(chan struct{})
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		<-blocked
	})
	t.Cleanup(func() { close(blocked) })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, _, err := svc.Search(ctx, SearchOptions{Query: "q"})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error once the context expired, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Search ignored context cancellation and blocked")
	}
}

// TestSearchMissingAPIKey asserts the key is checked before any request is
// made — a keyless deploy should fail loudly, not hit Brave anonymously.
func TestSearchMissingAPIKey(t *testing.T) {
	var called bool
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	svc.apiKey = ""

	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "golang", Limit: 10})
	if err == nil {
		t.Fatal("expected an error for the missing API key, got nil")
	}
	if !strings.Contains(err.Error(), "api key") {
		t.Errorf("error = %v, want it to mention the api key", err)
	}
	if called {
		t.Error("Search contacted the upstream despite having no API key")
	}
}

// TestExtractDomain tests URL domain extraction
func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{name: "https url with path", url: "https://golang.org/doc", expected: "golang.org"},
		{name: "subdomain is preserved", url: "https://www.github.com/golang/go", expected: "www.github.com"},
		{name: "bare host", url: "https://github.com", expected: "github.com"},
		{name: "port is stripped", url: "http://localhost:8080/path", expected: "localhost"},
		{name: "empty string", url: "", expected: ""},
		{name: "no scheme yields no host", url: "example.com/path", expected: ""},
		{name: "unparseable url", url: "http://[::1", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// TestNewBraveService asserts the constructor wires the defaults Search
// depends on, rather than just returning non-nil.
func TestNewBraveService(t *testing.T) {
	svc := NewBraveService("test-key-123")

	if svc.apiKey != "test-key-123" {
		t.Errorf("apiKey = %q, want %q", svc.apiKey, "test-key-123")
	}
	if svc.client == nil {
		t.Fatal("client is nil")
	}
	if svc.client.Timeout == 0 {
		t.Error("client has no timeout; a hung upstream would block the request forever")
	}
	if svc.limiter == nil {
		t.Error("limiter is nil; Search would panic")
	}
	if svc.endpoint != braveEndpoint {
		t.Errorf("endpoint = %q, want %q", svc.endpoint, braveEndpoint)
	}
}

// TestBraveServiceImplementsService is a compile-time assertion that the
// concrete type still satisfies the interface handlers depend on.
func TestBraveServiceImplementsService(t *testing.T) {
	var _ Service = (*BraveService)(nil)
}
