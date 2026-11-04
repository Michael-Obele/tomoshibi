package robots

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAllowed(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		agent  string
		path   string
		expect bool
	}{
		{
			name:   "longest match wins",
			body:   "User-agent: *\nDisallow: /admin\nAllow: /admin/public\n",
			agent:  "tomoshibi",
			path:   "/admin/public/page",
			expect: true,
		},
		{
			name:   "disallow still applies elsewhere",
			body:   "User-agent: *\nDisallow: /admin\nAllow: /admin/public\n",
			agent:  "tomoshibi",
			path:   "/admin/private",
			expect: false,
		},
		{
			name:   "end anchor stops at the exact path",
			body:   "User-agent: *\nDisallow: /*.pdf$\n",
			agent:  "tomoshibi",
			path:   "/docs/file.pdf",
			expect: false,
		},
		{
			name:   "end anchor does not match a query suffix",
			body:   "User-agent: *\nDisallow: /*.pdf$\n",
			agent:  "tomoshibi",
			path:   "/docs/file.pdf?download=1",
			expect: true,
		},
		{
			name:   "wildcard spans slashes",
			body:   "User-agent: *\nDisallow: /*/print$\n",
			agent:  "tomoshibi",
			path:   "/blog/2026/print",
			expect: false,
		},
		{
			name:   "specific group replaces the wildcard group",
			body:   "User-agent: *\nDisallow: /\n\nUser-agent: tomoshibi\nDisallow: /private\n",
			agent:  "tomoshibi",
			path:   "/public",
			expect: true,
		},
		{
			name:   "specific group still blocks its own rules",
			body:   "User-agent: *\nDisallow: /\n\nUser-agent: tomoshibi\nDisallow: /private\n",
			agent:  "tomoshibi",
			path:   "/private/x",
			expect: false,
		},
		{
			name:   "equal specificity tie goes to allow",
			body:   "User-agent: *\nDisallow: /page\nAllow: /page\n",
			agent:  "tomoshibi",
			path:   "/page",
			expect: true,
		},
		{
			name:   "empty disallow allows everything",
			body:   "User-agent: *\nDisallow:\n",
			agent:  "tomoshibi",
			path:   "/anything",
			expect: true,
		},
		{
			name:   "no matching group means allowed",
			body:   "User-agent: googlebot\nDisallow: /\n",
			agent:  "tomoshibi",
			path:   "/page",
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := Parse([]byte(tt.body), tt.agent)
			if got := rules.Allowed(tt.path); got != tt.expect {
				t.Errorf("Allowed(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

func TestDelayDirectives(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		want  time.Duration
		rules func(*Rules) time.Duration
	}{
		{
			name:  "crawl-delay seconds",
			body:  "User-agent: *\nCrawl-delay: 2\n",
			want:  2 * time.Second,
			rules: (*Rules).CrawlDelay,
		},
		{
			name:  "crawl-delay decimal",
			body:  "User-agent: *\nCrawl-delay: 0.5\n",
			want:  500 * time.Millisecond,
			rules: (*Rules).CrawlDelay,
		},
		{
			name:  "request-rate defaults to seconds",
			body:  "User-agent: *\nRequest-rate: 1/5\n",
			want:  5 * time.Second,
			rules: (*Rules).RequestRate,
		},
		{
			name:  "request-rate with minute unit",
			body:  "User-agent: *\nRequest-rate: 2/1m\n",
			want:  30 * time.Second,
			rules: (*Rules).RequestRate,
		},
		{
			name:  "strictest directive wins",
			body:  "User-agent: *\nCrawl-delay: 2\nRequest-rate: 1/10\n",
			want:  10 * time.Second,
			rules: (*Rules).Delay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := Parse([]byte(tt.body), "tomoshibi")
			if got := tt.rules(rules); got != tt.want {
				t.Errorf("delay = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCache_FetchesOnceAndHandlesMissingFile(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
	}))
	defer srv.Close()

	cache := NewCache(srv.Client())
	for i := 0; i < 3; i++ {
		rules, err := cache.For(context.Background(), srv.URL+"/page")
		if err != nil {
			t.Fatalf("For: %v", err)
		}
		if rules.Allowed("/private/x") {
			t.Fatal("/private should be disallowed")
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("robots.txt fetched %d times, want 1 (cache miss only)", got)
	}
}

func TestCache_MissingFileAllowsAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rules, err := NewCache(srv.Client()).For(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if !rules.Allowed("/anything") {
		t.Fatal("a 404 robots.txt must mean no restrictions")
	}
}

func TestCache_ServerErrorIsNotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := NewCache(srv.Client())
	if _, err := cache.For(context.Background(), srv.URL); err == nil {
		t.Fatal("expected an error for a 5xx robots.txt")
	}
	if _, err := cache.For(context.Background(), srv.URL); err == nil {
		t.Fatal("expected an error on the second attempt")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("5xx response was cached: %d fetches, want 2", got)
	}
}
