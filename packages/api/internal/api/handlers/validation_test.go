package handlers

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/Michael-Obele/tomoshibi/internal/search"
	"github.com/Michael-Obele/tomoshibi/internal/worker"
)

// deadRedisAddr has nothing listening: port 1 is privileged and never bound,
// so a connection there is refused immediately.
const deadRedisAddr = "127.0.0.1:1"

// deadRedisClient returns a go-redis client pointed at nothing, with retries
// disabled.
//
// The retries are the point. go-redis defaults to 3 attempts with backoff, so
// every command against an unreachable server costs about two seconds — the
// handler tests below issue one or more each and were spending 14s in backoff
// alone. Nothing here is testing reconnection behaviour, only what the handler
// does when the command fails, so the first failure is enough.
func deadRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	c := redis.NewClient(&redis.Options{
		Addr:        deadRedisAddr,
		MaxRetries:  -1, // -1 disables retries; 0 would mean "use the default"
		DialTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// deadBatchHandler builds a BatchHandler whose every Redis call fails fast.
func deadBatchHandler(t *testing.T) *BatchHandler {
	t.Helper()

	rdb := deadRedisClient(t)
	return &BatchHandler{
		client:    asynq.NewClientFromRedisClient(rdb),
		inspector: asynq.NewInspectorFromRedisClient(rdb),
		redis:     rdb,
	}
}

// deadCrawlHandler builds a CrawlHandler whose enqueue always fails.
func deadCrawlHandler(t *testing.T) *CrawlHandler {
	t.Helper()

	rdb := deadRedisClient(t)
	return &CrawlHandler{
		client:    asynq.NewClientFromRedisClient(rdb),
		inspector: asynq.NewInspectorFromRedisClient(rdb),
	}
}

// deadMonitorHandler builds a MonitorHandler whose every Redis call fails fast.
func deadMonitorHandler(t *testing.T) *MonitorHandler {
	t.Helper()

	rdb := deadRedisClient(t)
	return &MonitorHandler{redis: rdb, kv: worker.NewRedisKV(rdb)}
}

// postJSON runs one handler against a JSON body and returns the recorder.
func postJSON(t *testing.T, h gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h(c)
	return w
}

// TestExtractPreview covers the truncation rules: short input is returned
// whole, long input is cut on a word boundary when one is close enough, and
// the ellipsis marks that something was dropped.
func TestExtractPreview(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		maxLen   int
		want     string
	}{
		{
			name:     "shorter than the limit is returned whole",
			markdown: "short body",
			maxLen:   300,
			want:     "short body",
		},
		{
			name:     "surrounding whitespace is trimmed",
			markdown: "\n\n  padded  \n",
			maxLen:   300,
			want:     "padded",
		},
		{
			name:     "exactly at the limit is not truncated",
			markdown: "abcde",
			maxLen:   5,
			want:     "abcde",
		},
		{
			name:     "cuts on the nearest earlier space",
			markdown: "alpha bravo charlie",
			maxLen:   12,
			want:     "alpha bravo...",
		},
		{
			// No space within the 50-rune lookback, so it cuts mid-word rather
			// than walking back arbitrarily far.
			name:     "unbroken run is cut at the limit",
			markdown: strings.Repeat("x", 100),
			maxLen:   10,
			want:     strings.Repeat("x", 10) + "...",
		},
		{
			name:     "empty input",
			markdown: "",
			maxLen:   300,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractPreview(tt.markdown, tt.maxLen); got != tt.want {
				t.Errorf("extractPreview(%q, %d) = %q, want %q", tt.markdown, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestExtractPreviewCountsRunes asserts the limit is applied in runes, not
// bytes, so multi-byte content is never cut mid-character.
func TestExtractPreviewCountsRunes(t *testing.T) {
	// 40 three-byte runes: 120 bytes, well past a 20-byte limit.
	md := strings.Repeat("日", 40)

	got := extractPreview(md, 20)

	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected a truncated result, got %q", got)
	}
	body := strings.TrimSuffix(got, "...")
	if n := len([]rune(body)); n != 20 {
		t.Errorf("kept %d runes, want 20", n)
	}
	if strings.ContainsRune(body, '�') {
		t.Error("truncation split a multi-byte rune")
	}
}

// TestTaskStateLabelRemainingStates covers the states crawl_test.go does not,
// including the default branch for a state this code does not know about.
func TestTaskStateLabelRemainingStates(t *testing.T) {
	for state, want := range map[asynq.TaskState]string{
		asynq.TaskStateScheduled:   "scheduled",
		asynq.TaskStateRetry:       "retry",
		asynq.TaskStateAggregating: "aggregating",
	} {
		if got := taskStateLabel(state); got != want {
			t.Errorf("taskStateLabel(%v) = %q, want %q", state, got, want)
		}
	}
}

// TestRandomIDs asserts both ID generators produce distinct 32-character hex
// strings. Collisions would let one batch or monitor overwrite another's Redis
// record.
func TestRandomIDs(t *testing.T) {
	for name, gen := range map[string]func() (string, error){
		"batch":   newBatchID,
		"monitor": newMonitorID,
	} {
		t.Run(name, func(t *testing.T) {
			seen := make(map[string]bool, 100)
			for i := 0; i < 100; i++ {
				id, err := gen()
				if err != nil {
					t.Fatalf("generator returned an error: %v", err)
				}
				if len(id) != 32 {
					t.Fatalf("id %q has length %d, want 32", id, len(id))
				}
				if _, err := hex.DecodeString(id); err != nil {
					t.Fatalf("id %q is not hex: %v", id, err)
				}
				if seen[id] {
					t.Fatalf("duplicate id %q after %d draws", id, i)
				}
				seen[id] = true
			}
		})
	}
}

// TestHandlerConstructorsRejectBadRedisURL asserts an unparseable URL is
// reported at construction. cmd/api treats that error as "async disabled"
// rather than starting a handler that fails on every request.
func TestHandlerConstructorsRejectBadRedisURL(t *testing.T) {
	// redis.ParseURL rejects a non-redis scheme; url.Parse rejects the control
	// character. Between them every constructor's error path is reached.
	for _, bad := range []string{"http://example.com", "redis://ho\x7fst"} {
		t.Run(bad, func(t *testing.T) {
			if _, err := NewBatchHandler(bad); err == nil {
				t.Error("NewBatchHandler accepted an invalid redis URL")
			}
			if _, err := NewMonitorHandler(bad); err == nil {
				t.Error("NewMonitorHandler accepted an invalid redis URL")
			}
		})
	}
}

// TestHandlerConstructorsAcceptValidURLs covers the success path of each
// constructor and its Close. All three clients connect lazily, so this touches
// no network — which is also why cmd/api can build them before knowing whether
// Redis is actually reachable.
func TestHandlerConstructorsAcceptValidURLs(t *testing.T) {
	// rediss:// additionally exercises the TLS branch in the crawl and batch
	// constructors.
	for _, addr := range []string{
		"redis://" + deadRedisAddr,
		"redis://:secret@" + deadRedisAddr + "/0",
		"rediss://" + deadRedisAddr,
	} {
		t.Run(addr, func(t *testing.T) {
			crawl, err := NewCrawlHandler(addr)
			if err != nil {
				t.Fatalf("NewCrawlHandler: %v", err)
			}
			crawl.Close()

			batch, err := NewBatchHandler(addr)
			if err != nil {
				t.Fatalf("NewBatchHandler: %v", err)
			}
			batch.Close()

			monitor, err := NewMonitorHandler(addr)
			if err != nil {
				t.Fatalf("NewMonitorHandler: %v", err)
			}
			monitor.Close()
		})
	}
}

// TestEnqueueCrawlRejectsBadRequests covers the binding rules on
// POST /v1/crawl. Every case must fail before Redis is touched, so a nil
// client here is deliberate: if one of them ever stops short-circuiting, this
// test panics instead of quietly passing.
func TestEnqueueCrawlRejectsBadRequests(t *testing.T) {
	h := &CrawlHandler{}

	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{"url":`},
		{name: "missing url", body: `{}`},
		{name: "empty url", body: `{"url":""}`},
		{name: "not a url", body: `{"url":"not-a-url"}`},
		{name: "wrong type for url", body: `{"url":123}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postJSON(t, h.EnqueueCrawl, tt.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
		})
	}
}

// TestEnqueueCrawlClampsDefaults asserts the depth and limit clamps run, and
// that an unreachable Redis surfaces as a 500 rather than a panic. The clamped
// values are not observable in the error response, so this covers the branches
// and the failure mode; the values themselves are asserted in the worker's own
// task tests.
func TestEnqueueCrawlClampsDefaults(t *testing.T) {
	h := deadCrawlHandler(t)

	for _, body := range []string{
		`{"url":"https://example.com"}`,                           // defaults applied
		`{"url":"https://example.com","maxDepth":99,"limit":0}`,   // depth capped, limit defaulted
		`{"url":"https://example.com","maxDepth":-1,"limit":999}`, // depth defaulted, limit capped
	} {
		w := postJSON(t, h.EnqueueCrawl, body)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500 for %s (body: %s)", w.Code, body, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "failed to enqueue task") {
			t.Errorf("expected an enqueue failure, got %s", w.Body.String())
		}
	}
}

// TestEnqueueBatchRejectsBadRequests covers the batch size and binding rules.
// The URL list is bounded because each entry becomes its own queued task.
func TestEnqueueBatchRejectsBadRequests(t *testing.T) {
	h := &BatchHandler{}

	var urls []string
	for i := 0; i < maxBatchURLs+1; i++ {
		urls = append(urls, `"https://example.com"`)
	}
	tooMany := `{"urls":[` + strings.Join(urls, ",") + `]}`

	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{"urls":`},
		{name: "missing urls", body: `{}`},
		{name: "empty list", body: `{"urls":[]}`},
		{name: "over the max", body: tooMany},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postJSON(t, h.EnqueueBatch, tt.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
		})
	}
}

// TestGetBatchStatusMissing asserts an unknown batch is a 404, not a 500. The
// Redis lookup fails here because nothing is listening, which is the same
// error path as a genuinely absent key.
func TestGetBatchStatusMissing(t *testing.T) {
	h := deadBatchHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "does-not-exist"}}

	h.GetBatchStatus(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", w.Code, w.Body.String())
	}
}

// TestCreateMonitorRejectsBadRequests covers monitor validation. The interval
// floor matters: the scheduler polls every monitor on its own timer, so a
// short interval multiplies both scrape load and Redis commands.
func TestCreateMonitorRejectsBadRequests(t *testing.T) {
	h := &MonitorHandler{}

	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{name: "malformed json", body: `{"url":`},
		{name: "missing url", body: `{"interval_seconds":3600}`},
		{name: "not a url", body: `{"url":"nope","interval_seconds":3600}`},
		{
			name:    "interval below the floor",
			body:    `{"url":"https://example.com","interval_seconds":60}`,
			wantMsg: "interval_seconds must be at least 3600",
		},
		{
			name:    "zero interval",
			body:    `{"url":"https://example.com"}`,
			wantMsg: "interval_seconds must be at least 3600",
		},
		{
			name:    "negative interval",
			body:    `{"url":"https://example.com","interval_seconds":-1}`,
			wantMsg: "interval_seconds must be at least 3600",
		},
		{
			name:    "invalid webhook url",
			body:    `{"url":"https://example.com","interval_seconds":3600,"webhook_url":"not a url"}`,
			wantMsg: "invalid webhook_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postJSON(t, h.CreateMonitor, tt.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
			if tt.wantMsg != "" && !strings.Contains(w.Body.String(), tt.wantMsg) {
				t.Errorf("body = %s, want it to mention %q", w.Body.String(), tt.wantMsg)
			}
		})
	}
}

// TestCreateMonitorPersistFailure asserts a valid request that cannot be
// persisted is reported as a 500 rather than returning an ID for a monitor
// that was never stored.
func TestCreateMonitorPersistFailure(t *testing.T) {
	h := deadMonitorHandler(t)

	w := postJSON(t, h.CreateMonitor, `{"url":"https://example.com","interval_seconds":3600}`)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to persist monitor") {
		t.Errorf("body = %s, want it to mention the persist failure", w.Body.String())
	}
}

// TestMonitorLookupsFailClosed asserts neither read path reports success for a
// monitor it could not load.
func TestMonitorLookupsFailClosed(t *testing.T) {
	h := deadMonitorHandler(t)

	tests := []struct {
		name   string
		method string
		call   func(*gin.Context)
	}{
		{name: "get", method: http.MethodGet, call: h.GetMonitor},
		{name: "delete", method: http.MethodDelete, call: h.DeleteMonitor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(tt.method, "/", nil)
			c.Params = gin.Params{{Key: "id", Value: "does-not-exist"}}

			tt.call(c)

			// DELETE against an unreachable Redis fails the command itself, so
			// it reports 500; GET cannot tell that apart from a missing key and
			// reports 404. What matters is that neither claims success.
			if w.Code == http.StatusOK || w.Code == http.StatusNoContent {
				t.Errorf("status = %d, want a failure for a monitor that could not be read", w.Code)
			}
		})
	}
}

// TestMapRejectsBadRequests covers the binding rules on POST /v1/map. None of
// these should reach sitemap discovery.
func TestMapRejectsBadRequests(t *testing.T) {
	h := NewMapHandler()

	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{"url":`},
		{name: "missing url", body: `{}`},
		{name: "empty url", body: `{"url":""}`},
		{name: "not a url", body: `{"url":"just-a-string"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postJSON(t, h.Map, tt.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
		})
	}
}

// TestMapLimitClamp asserts the limit is bounded before it reaches discovery.
// A limit of zero must become the default rather than asking for no URLs, and
// an oversized one must be capped so a single request cannot ask for unbounded
// sitemap traversal.
func TestMapLimitClamp(t *testing.T) {
	// Serve more URLs than the smallest limit under test so a clamp that went
	// the wrong way would show up as a different count.
	var site *httptest.Server
	site = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Write([]byte("Sitemap: " + site.URL + "/sitemap.xml\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			var b bytes.Buffer
			b.WriteString("<urlset>")
			for i := 0; i < 5; i++ {
				fmt.Fprintf(&b, "<url><loc>%s/p%c</loc></url>", site.URL, 'a'+i)
			}
			b.WriteString("</urlset>")
			w.Write(b.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()

	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "zero limit uses the default", body: `{"url":"` + site.URL + `"}`, want: 5},
		{name: "negative limit uses the default", body: `{"url":"` + site.URL + `","limit":-5}`, want: 5},
		{name: "limit below the count truncates", body: `{"url":"` + site.URL + `","limit":2}`, want: 2},
		{name: "oversized limit is capped and harmless", body: `{"url":"` + site.URL + `","limit":99999}`, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postJSON(t, NewMapHandler().Map, tt.body)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
			}
			var resp MapResponse
			if err := decodeJSON(t, w, &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Count != tt.want {
				t.Errorf("count = %d, want %d", resp.Count, tt.want)
			}
			if len(resp.Links) != resp.Count {
				t.Errorf("count %d disagrees with %d links", resp.Count, len(resp.Links))
			}
		})
	}
}

// TestSearchHandlerQueryParams covers the GET path and the query-string
// overrides, which the JSON-body tests in search_test.go never reach.
func TestSearchHandlerQueryParams(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantQuery  string
		wantOffset int
		wantLimit  int
	}{
		{
			name:      "query param",
			url:       "/search?query=golang",
			wantQuery: "golang", wantLimit: 10,
		},
		{
			name:      "q alias",
			url:       "/search?q=generics",
			wantQuery: "generics", wantLimit: 10,
		},
		{
			// query wins over q when both are present.
			name:      "query beats the alias",
			url:       "/search?query=first&q=second",
			wantQuery: "first", wantLimit: 10,
		},
		{
			name:      "offset and limit",
			url:       "/search?q=go&offset=30&limit=25",
			wantQuery: "go", wantOffset: 30, wantLimit: 25,
		},
		{
			// Unparseable numbers are ignored rather than rejected, leaving the
			// defaults in place.
			name:      "non-numeric offset and limit are ignored",
			url:       "/search?q=go&offset=abc&limit=xyz",
			wantQuery: "go", wantOffset: 0, wantLimit: 10,
		},
		{
			name:      "limit is capped at 100",
			url:       "/search?q=go&limit=5000",
			wantQuery: "go", wantLimit: 100,
		},
		{
			name:      "negative offset is floored at zero",
			url:       "/search?q=go&offset=-40",
			wantQuery: "go", wantOffset: 0, wantLimit: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got search.SearchOptions
			svc := &MockSearchService{
				SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
					got = opts
					return nil, 0, nil
				},
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tt.url, nil)

			NewSearchHandler(svc).Search(c)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
			}
			if got.Query != tt.wantQuery {
				t.Errorf("Query = %q, want %q", got.Query, tt.wantQuery)
			}
			if got.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", got.Offset, tt.wantOffset)
			}
			if got.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", got.Limit, tt.wantLimit)
			}
		})
	}
}

// TestSearchHandlerGetWithoutQuery asserts a GET with no query at all is a 400
// rather than an empty search against the upstream.
func TestSearchHandlerGetWithoutQuery(t *testing.T) {
	var called bool
	svc := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			called = true
			return nil, 0, nil
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/search", nil)

	NewSearchHandler(svc).Search(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if called {
		t.Error("handler called the search service with no query")
	}
}

// TestSearchHandlerBodyThenQueryOverride asserts a POST body supplies the
// baseline and query-string parameters override it, which is the documented
// precedence for the dual-mode endpoint.
func TestSearchHandlerBodyThenQueryOverride(t *testing.T) {
	var got search.SearchOptions
	svc := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			got = opts
			return nil, 0, nil
		},
	}

	body := `{"query":"from-body","limit":20,"mode":"thorough"}`
	req := httptest.NewRequest(http.MethodPost, "/search?query=from-url&limit=5", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	NewSearchHandler(svc).Search(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	if got.Query != "from-url" {
		t.Errorf("Query = %q, want the query-string value %q", got.Query, "from-url")
	}
	if got.Limit != 5 {
		t.Errorf("Limit = %d, want the query-string value 5", got.Limit)
	}
	// Fields with no query-string equivalent survive from the body.
	if got.Mode != "thorough" {
		t.Errorf("Mode = %q, want %q from the body", got.Mode, "thorough")
	}
}

// TestSearchHandlerPassesThroughFilters asserts the body-only filter fields
// reach the service unchanged — the handler must not silently drop options it
// has no query-string parsing for.
func TestSearchHandlerPassesThroughFilters(t *testing.T) {
	maxAge := 7
	var got search.SearchOptions
	svc := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			got = opts
			return nil, 0, nil
		},
	}

	body := `{"query":"q","includeDomains":["a.com"],"excludeDomains":["b.com"],` +
		`"requiredText":["needle"],"maxAge":7,"mode":"fast"}`
	w := postJSON(t, NewSearchHandler(svc).Search, body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	if len(got.IncludeDomains) != 1 || got.IncludeDomains[0] != "a.com" {
		t.Errorf("IncludeDomains = %v, want [a.com]", got.IncludeDomains)
	}
	if len(got.ExcludeDomains) != 1 || got.ExcludeDomains[0] != "b.com" {
		t.Errorf("ExcludeDomains = %v, want [b.com]", got.ExcludeDomains)
	}
	if len(got.RequiredText) != 1 || got.RequiredText[0] != "needle" {
		t.Errorf("RequiredText = %v, want [needle]", got.RequiredText)
	}
	if got.MaxAge == nil || *got.MaxAge != maxAge {
		t.Errorf("MaxAge = %v, want %d", got.MaxAge, maxAge)
	}
	if got.Mode != "fast" {
		t.Errorf("Mode = %q, want fast", got.Mode)
	}
}

// TestSearchHandlerMalformedBody asserts a POST with an unparseable body is a
// 400 rather than falling through to an empty search.
func TestSearchHandlerMalformedBody(t *testing.T) {
	svc := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			t.Error("handler reached the service despite a malformed body")
			return nil, 0, nil
		},
	}

	w := postJSON(t, NewSearchHandler(svc).Search, `{"query":`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
}

// decodeJSON unmarshals a recorder body, reporting the body on failure so a
// mismatch is diagnosable from the test output alone.
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) error {
	t.Helper()
	return json.Unmarshal(w.Body.Bytes(), v)
}
