package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Heavy-browsing load harness for Cinder.
//
// Unlike a smoke test, this hits real-world URLs (no example.com) with real
// concurrency to surface 404s, timeouts, DNS failures, and Chrome contention
// under load. It is deliberately diagnostic: every phase reports per-endpoint
// status distributions, latency percentiles, and classified errors so a
// regression shows up as a number, not a vibe.

var (
	baseURL     string
	concurrency int
	duration    time.Duration
)

var heavyURLs = []string{
	"https://en.wikipedia.org/wiki/Go_(programming_language)",
	"https://news.ycombinator.com",
	"https://github.com/golang/go",
	"https://developer.mozilla.org/en-US/docs/Web/JavaScript",
	"https://www.bbc.com/news",
	"https://www.reddit.com/r/golang/",
	"https://stackoverflow.com/questions/tagged/go",
	"https://go.dev/blog",
	"https://www.theverge.com/tech",
	"https://medium.com/tag/golang",
	"https://www.nytimes.com",
	"https://www.cnn.com",
	"https://www.wikipedia.org",
	"https://www.npmjs.com/package/react",
	"https://docs.python.org/3/",
}

// stat aggregates one endpoint's results.
type stat struct {
	counts    map[int]int // HTTP status -> count
	errs      map[string]int
	latencies []time.Duration
	mu        sync.Mutex
}

func newStat() *stat {
	return &stat{counts: map[int]int{}, errs: map[string]int{}}
}

func (s *stat) record(status int, lat time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencies = append(s.latencies, lat)
	if err != nil {
		s.errs[classifyErr(err)]++
		return
	}
	s.counts[status]++
}

// percentiles returns p50/p95/p99 of recorded latencies.
func (s *stat) percentiles() (p50, p95, p99 time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.latencies) == 0 {
		return 0, 0, 0
	}
	lats := make([]time.Duration, len(s.latencies))
	copy(lats, s.latencies)
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	at := func(p float64) time.Duration {
		idx := int(float64(len(lats)-1) * p)
		return lats[idx]
	}
	return at(0.50), at(0.95), at(0.99)
}

func (s *stat) report(name string) {
	p50, p95, p99 := s.percentiles()
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Printf("  %-28s n=%-5d 2xx=%-4d 4xx=%-4d 5xx=%-4d err=%-4d  p50=%-8s p95=%-8s p99=%-8s\n",
		name, len(s.latencies),
		s.counts[200]+s.counts[201]+s.counts[202]+s.counts[204],
		s.counts[400]+s.counts[404]+s.counts[429],
		s.counts[500]+s.counts[502]+s.counts[503],
		len(s.errs),
		p50.Round(time.Millisecond), p95.Round(time.Millisecond), p99.Round(time.Millisecond))
	if len(s.errs) > 0 {
		for cls, n := range s.errs {
			fmt.Printf("      error[%s]=%d\n", cls, n)
		}
	}
}

// classifyErr buckets an error into a stable class for reporting.
func classifyErr(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no such host"),
		strings.Contains(msg, "server misbehaving"),
		strings.Contains(msg, "Temporary failure in name resolution"),
		strings.Contains(msg, "i/o timeout") && strings.Contains(msg, "lookup"):
		return "dns"
	case strings.Contains(msg, "connection refused"):
		return "conn-refused"
	case strings.Contains(msg, "connection reset"):
		return "conn-reset"
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "Client.Timeout"),
		strings.Contains(msg, "deadline exceeded"):
		return "timeout"
	case strings.Contains(msg, "EOF"):
		return "eof"
	default:
		return "other"
	}
}

func main() {
	flag.StringVar(&baseURL, "base", "http://localhost:8080", "Cinder base URL")
	flag.IntVar(&concurrency, "concurrency", 20, "concurrent workers for load phases")
	flag.DurationVar(&duration, "duration", 60*time.Second, "sustained mixed-load duration")
	flag.Parse()

	client := &http.Client{Timeout: 45 * time.Second}

	fmt.Println("=== Cinder Heavy-Browsing Load Harness ===")
	fmt.Printf("Base: %s | concurrency: %d | sustained: %s\n", baseURL, concurrency, duration)
	fmt.Printf("Targets: %d real URLs, not example.com\n", len(heavyURLs))
	fmt.Println()

	// 1. Health probes — the "is the router even there" check.
	fmt.Println("--- 1. Health probes ---")
	healthOK := true
	for _, p := range []string{"/", "/health", "/v1/ping"} {
		code := probe(client, "GET", p, nil)
		if code != 200 {
			healthOK = false
			fmt.Printf("  !! %s returned %d — the running binary is stale or the router is broken\n", p, code)
		}
	}
	fmt.Println()

	// 2. Single heavy scrape sanity.
	fmt.Println("--- 2. Single heavy scrape sanity ---")
	probeScrape(client, heavyURLs[0])
	fmt.Println()

	// 3. Concurrent scrape burst.
	fmt.Printf("--- 3. Concurrent scrape burst (%d parallel, heavy URLs) ---\n", concurrency)
	burstStat := newStat()
	runBurst(client, concurrency, heavyURLs, burstStat)
	burstStat.report("scrape burst")
	fmt.Println()

	// 4. Crawl lifecycle with false-404 detection.
	fmt.Println("--- 4. Crawl lifecycle + false-404 detection ---")
	testCrawlLifecycle(client)
	fmt.Println()

	// 5. Sustained mixed workload.
	fmt.Printf("--- 5. Sustained mixed workload (%d parallel, %s) ---\n", concurrency, duration)
	mixed := runMixed(client, concurrency, duration)
	for name, s := range mixed {
		s.report(name)
	}
	fmt.Println()

	// 6. False-404 audit: real IDs hammered under load must NEVER 404.
	fmt.Println("--- 6. False-404 audit (valid IDs under load) ---")
	auditFalse404s(client, concurrency)
	fmt.Println()

	// 7. Expected-404 audit (routes that SHOULD 404).
	fmt.Println("--- 7. Expected-404 audit ---")
	audit404s(client)
	fmt.Println()

	fmt.Println("=== Harness done ===")
	if !healthOK {
		fmt.Println("!! Health probes failed — rebuild/redeploy the running binary before trusting these numbers.")
		os.Exit(1)
	}
}

func probe(client *http.Client, method, path string, body any) int {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, baseURL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  %s %s -> ERR: %v\n", method, path, err)
		return 0
	}
	defer resp.Body.Close()
	fmt.Printf("  %s %s -> %d %s\n", method, path, resp.StatusCode, http.StatusText(resp.StatusCode))
	if resp.StatusCode == 404 {
		bb, _ := io.ReadAll(resp.Body)
		fmt.Printf("      body: %s\n", string(bb))
	}
	return resp.StatusCode
}

func probeScrape(client *http.Client, url string) {
	body := map[string]string{"url": url, "mode": "smart"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/v1/scrape", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  scrape %s -> ERR: %v\n", url, err)
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	elapsed := time.Since(start)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	md, _ := out["markdown"].(string)
	fmt.Printf("  scrape %s -> %d in %s (markdown %d chars)\n", url, resp.StatusCode, elapsed.Round(time.Millisecond), len(md))
	if resp.StatusCode != 200 {
		fmt.Printf("      body: %.500s\n", string(data))
	}
}

func runBurst(client *http.Client, n int, urls []string, st *stat) {
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			u := urls[idx%len(urls)]
			body := map[string]string{"url": u}
			b, _ := json.Marshal(body)
			req, _ := http.NewRequest("POST", baseURL+"/v1/scrape", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			t0 := time.Now()
			resp, err := client.Do(req)
			lat := time.Since(t0)
			if err != nil {
				st.record(0, lat, err)
				fmt.Printf("    [%d] %s -> ERR %v (%s)\n", idx, u, err, lat.Round(time.Millisecond))
				return
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			st.record(resp.StatusCode, lat, nil)
			if resp.StatusCode != 200 {
				fmt.Printf("    [%d] %s -> %d (%s)\n", idx, u, resp.StatusCode, lat.Round(time.Millisecond))
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("  Burst finished in %s\n", time.Since(start).Round(time.Millisecond))
}

// testCrawlLifecycle enqueues a crawl and polls until it reaches a terminal
// state. A 404 on a valid ID within retention is a FALSE 404 — the task was
// deleted or the inspector failed — and is flagged loudly.
func testCrawlLifecycle(client *http.Client) {
	body := map[string]any{"url": "https://en.wikipedia.org/wiki/Go_(programming_language)", "limit": 3, "maxDepth": 1}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/v1/crawl", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  crawl enqueue -> ERR %v\n", err)
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	fmt.Printf("  crawl enqueue -> %d %s\n", resp.StatusCode, string(data))
	if resp.StatusCode != 202 {
		return
	}
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	id, _ := out["id"].(string)
	if id == "" {
		fmt.Println("  no id returned")
		return
	}
	// Poll up to 2 minutes; crawl of a heavy site can take a while.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		req2, _ := http.NewRequest("GET", baseURL+"/v1/crawl/"+id, nil)
		resp2, err := client.Do(req2)
		if err != nil {
			fmt.Printf("  poll -> ERR %v\n", err)
			continue
		}
		d2, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		var st map[string]any
		_ = json.Unmarshal(d2, &st)
		state, _ := st["state"].(string)
		fmt.Printf("  poll -> %d state=%s %s\n", resp2.StatusCode, state, truncate(string(d2), 220))
		if resp2.StatusCode == 404 {
			fmt.Printf("  !! FALSE 404 on valid crawl id=%s (within 7-day retention) — task deleted or inspector miss\n", id)
			return
		}
		if resp2.StatusCode == 503 {
			fmt.Printf("  !! 503 (transient) on valid crawl id=%s — Redis/inspector unavailable under load\n", id)
			continue
		}
		if state == "completed" || state == "failed" {
			fmt.Printf("  crawl reached terminal state %q — no false 404\n", state)
			return
		}
	}
	fmt.Println("  crawl did not reach terminal state within 2m (may still be running)")
}

// runMixed drives scrape/search/map/static-scrape against the server for the
// given duration and returns per-endpoint stats.
func runMixed(client *http.Client, concurrency int, duration time.Duration) map[string]*stat {
	stats := map[string]*stat{
		"/v1/scrape":        newStat(),
		"/v1/scrape:static": newStat(),
		"/v1/search":        newStat(),
		"/v1/map":           newStat(),
	}
	end := time.Now().Add(duration)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			n := 0
			for time.Now().Before(end) {
				var path string
				var body any
				switch n % 4 {
				case 0:
					path = "/v1/scrape"
					body = map[string]string{"url": heavyURLs[(id+n)%len(heavyURLs)]}
				case 1:
					path = "/v1/search"
					body = map[string]string{"query": "golang concurrency"}
				case 2:
					path = "/v1/map"
					body = map[string]string{"url": heavyURLs[(id+n)%len(heavyURLs)]}
				case 3:
					path = "/v1/scrape"
					body = map[string]string{"url": heavyURLs[(id+n)%len(heavyURLs)], "mode": "static"}
				}
				statKey := path
				if n%4 == 3 {
					statKey = "/v1/scrape:static"
				}
				b, _ := json.Marshal(body)
				req, _ := http.NewRequest("POST", baseURL+path, bytes.NewReader(b))
				req.Header.Set("Content-Type", "application/json")
				t0 := time.Now()
				resp, err := client.Do(req)
				lat := time.Since(t0)
				if err != nil {
					stats[statKey].record(0, lat, err)
				} else {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					stats[statKey].record(resp.StatusCode, lat, nil)
				}
				n++
				time.Sleep(200 * time.Millisecond)
			}
		}(i)
	}
	wg.Wait()
	return stats
}

// auditFalse404s creates real crawl/batch/monitor records and hammers their
// status endpoints concurrently while ALSO running scrapes. Any 404 on a
// valid ID is a false negative and is flagged.
func auditFalse404s(client *http.Client, concurrency int) {
	// Create one batch (2 URLs) and one monitor.
	batchID := ""
	{
		body := map[string]any{"urls": []string{
			"https://en.wikipedia.org/wiki/Go_(programming_language)",
			"https://go.dev/blog",
		}}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", baseURL+"/v1/batch/scrape", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			d, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var out map[string]any
			_ = json.Unmarshal(d, &out)
			batchID, _ = out["batch_id"].(string)
			fmt.Printf("  created batch id=%s\n", batchID)
		} else {
			fmt.Printf("  batch create ERR: %v\n", err)
		}
	}
	monitorID := ""
	{
		body := map[string]any{"url": "https://go.dev/blog", "interval_seconds": 3600}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", baseURL+"/v1/monitor", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			d, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var out map[string]any
			_ = json.Unmarshal(d, &out)
			monitorID, _ = out["id"].(string)
			fmt.Printf("  created monitor id=%s\n", monitorID)
		} else {
			fmt.Printf("  monitor create ERR: %v\n", err)
		}
	}

	if batchID == "" && monitorID == "" {
		fmt.Println("  could not create any records — Redis may be down")
		return
	}

	// Hammer valid IDs concurrently with scrapes for ~20s.
	deadline := time.Now().Add(20 * time.Second)
	var wg sync.WaitGroup
	var false404, transient503, ok int64
	for i := 0; i < concurrency/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				var path string
				switch id % 3 {
				case 0:
					if batchID == "" {
						continue
					}
					path = "/v1/batch/" + batchID
				case 1:
					if monitorID == "" {
						continue
					}
					path = "/v1/monitor/" + monitorID
				default:
					// Also keep scraping to keep the server busy.
					body := map[string]string{"url": heavyURLs[(id+int(time.Now().Unix()))%len(heavyURLs)], "mode": "static"}
					b, _ := json.Marshal(body)
					req, _ := http.NewRequest("POST", baseURL+"/v1/scrape", bytes.NewReader(b))
					req.Header.Set("Content-Type", "application/json")
					resp, err := client.Do(req)
					if err == nil {
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
					time.Sleep(300 * time.Millisecond)
					continue
				}
				req, _ := http.NewRequest("GET", baseURL+path, nil)
				resp, err := client.Do(req)
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				switch resp.StatusCode {
				case 200:
					atomic.AddInt64(&ok, 1)
				case 404:
					atomic.AddInt64(&false404, 1)
				case 503:
					atomic.AddInt64(&transient503, 1)
				}
				time.Sleep(200 * time.Millisecond)
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("  valid-ID status polls: %d ok, %d FALSE-404, %d transient-503\n", ok, false404, transient503)
	if false404 > 0 {
		fmt.Printf("  !! %d FALSE 404s on valid IDs under load — the false-404 bug is still present\n", false404)
	} else if transient503 > 0 {
		fmt.Printf("  !! %d transient 503s — Redis/inspector degraded under load (correct behavior, but worth watching)\n", transient503)
	} else {
		fmt.Println("  no false 404s and no transient 503s on valid IDs — healthy")
	}
}

func audit404s(client *http.Client) {
	cases := []struct {
		method, path string
		expect       int
	}{
		{"GET", "/health", 200},
		{"GET", "/v1/ping", 200},
		{"GET", "/", 200},
		{"GET", "/v1/crawl/nonexistent-id-12345", 404},
		{"GET", "/v1/batch/nonexistent-id-12345", 404},
		{"GET", "/v1/monitor/nonexistent-id-12345", 404},
		{"POST", "/v1/scrape", 400},         // no body
		{"GET", "/v1/models", 404},          // not a cinder route — should be 404
		{"GET", "/swagger/index.html", 404}, // release mode
	}
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, baseURL+c.path, nil)
		if c.method == "POST" && c.path == "/v1/scrape" {
			req.Header.Set("Content-Type", "application/json")
			req.Body = io.NopCloser(bytes.NewReader([]byte(`{}`)))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  %s %s -> ERR %v (expect %d)\n", c.method, c.path, err, c.expect)
			continue
		}
		resp.Body.Close()
		mark := "OK"
		if resp.StatusCode != c.expect {
			mark = "UNEXPECTED"
		}
		fmt.Printf("  %s %s -> %d (expect %d) [%s]\n", c.method, c.path, resp.StatusCode, c.expect, mark)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
