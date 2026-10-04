package engines

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	http2 "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// browserUA is the profile-matched Chrome UA. Deliberately FIXED, not
// gofakeit-random: a UA that disagrees with the TLS profile is itself a
// bot signal, and the 2025-12 search test report traced DDG failures to
// random UA rotation (test_reports/search_feature_report.md).
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

// maxBodyBytes caps how much of a response we buffer.
const maxBodyBytes = 4 << 20

// response is a transport-agnostic HTTP result.
type response struct {
	Status int
	Body   []byte
}

// transportPool hands out fetchers per (profile, proxyURL) pair. net/http
// is used for profileless API/RSS engines (no fingerprint needed — those
// endpoints do not fingerprint and work today from a datacenter IP);
// tls-client gives HTML SERPs a browser JA3/JA4.
type transportPool struct {
	proxy *ProxyPool

	mu       sync.Mutex
	netCache map[string]*http.Client // key: proxyURL
	tlsCache map[string]tlsclient.HttpClient
}

func newTransportPool(p *ProxyPool) *transportPool {
	return &transportPool{
		proxy:    p,
		netCache: make(map[string]*http.Client),
		tlsCache: make(map[string]tlsclient.HttpClient),
	}
}

func (tp *transportPool) Do(ctx context.Context, profile, proxyGroup, method, rawURL, body string, headers map[string]string) (response, error) {
	proxyURL := ""
	if tp.proxy != nil && proxyGroup != "" {
		proxyURL = tp.proxy.Next(proxyGroup)
	}
	if profile == "" {
		return tp.doNet(ctx, proxyURL, method, rawURL, body, headers)
	}
	return tp.doTLS(ctx, profile, proxyURL, method, rawURL, body, headers)
}

func (tp *transportPool) doNet(ctx context.Context, proxyURL, method, rawURL, body string, headers map[string]string) (response, error) {
	client := tp.netClient(proxyURL)

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return response{}, fmt.Errorf("build request: %w", err)
	}
	applyHeaders(req.Header, headers)
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", browserUA)
	}

	resp, err := client.Do(req)
	if err != nil {
		return response{}, fmt.Errorf("request %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body, maxBodyBytes)
	if err != nil {
		return response{}, fmt.Errorf("read body: %w", err)
	}
	tp.countSpend(proxyURL, len(raw))
	return response{Status: resp.StatusCode, Body: raw}, nil
}

func (tp *transportPool) doTLS(ctx context.Context, profile, proxyURL, method, rawURL, body string, headers map[string]string) (response, error) {
	client, err := tp.tlsClient(profile, proxyURL)
	if err != nil {
		return response{}, err
	}

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http2.NewRequest(http2.MethodGet, rawURL, reader)
	if err != nil {
		return response{}, fmt.Errorf("build request: %w", err)
	}
	if method != "" && method != http2.MethodGet {
		req.Method = method
	}
	applyHeaders(req.Header, headers)
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", browserUA)
	}
	// Browser-like header order — tls-client emits the map iteration order
	// it finds here as the HTTP/2 pseudo + regular header order.
	req.Header[http2.PHeaderOrderKey] = []string{":method", ":authority", ":scheme", ":path"}
	req.Header[http2.HeaderOrderKey] = []string{
		"accept", "accept-language", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "upgrade-insecure-requests", "user-agent", "accept-encoding", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest", "referer", "content-type",
	}

	resp, err := client.Do(req)
	if err != nil {
		return response{}, fmt.Errorf("request %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body, maxBodyBytes)
	if err != nil {
		return response{}, fmt.Errorf("read body: %w", err)
	}
	tp.countSpend(proxyURL, len(raw))
	return response{Status: resp.StatusCode, Body: raw}, nil
}

func (tp *transportPool) netClient(proxyURL string) *http.Client {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if c, ok := tp.netCache[proxyURL]; ok {
		return c
	}
	transport := &http.Transport{
		MaxIdleConns:        32,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	c := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	tp.netCache[proxyURL] = c
	return c
}

func (tp *transportPool) tlsClient(profile, proxyURL string) (tlsclient.HttpClient, error) {
	key := profile + "|" + proxyURL
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if c, ok := tp.tlsCache[key]; ok {
		return c, nil
	}
	cp, ok := profiles.MappedTLSClients[profile]
	if !ok {
		if profile == "chrome" {
			cp = profiles.DefaultClientProfile
		} else {
			return nil, fmt.Errorf("unknown TLS profile %q", profile)
		}
	}
	opts := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(cp),
		tlsclient.WithTimeoutSeconds(20),
		tlsclient.WithCatchPanics(),
	}
	if proxyURL != "" {
		opts = append(opts, tlsclient.WithProxyUrl(proxyURL))
	}
	c, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), opts...)
	if err != nil {
		return nil, fmt.Errorf("create tls-client (%s): %w", profile, err)
	}
	tp.tlsCache[key] = c
	return c, nil
}

// countSpend feeds the proxy byte budget (no-op for direct requests).
func (tp *transportPool) countSpend(proxyURL string, n int) {
	if tp.proxy != nil && proxyURL != "" {
		tp.proxy.Spend(n)
	}
}

func applyHeaders(dst map[string][]string, headers map[string]string) {
	h := http.Header(dst) // net/http.Header and fhttp.Header share this shape
	for k, v := range headers {
		if v == "" {
			continue
		}
		h.Set(k, v)
	}
}

// engine fetcher wiring: render the request, run it through the pool,
// classify blocks, and parse the body.

type engineHTTP struct {
	spec    *Spec
	pool    *transportPool
	timeout time.Duration
	gate    *timeGate
}

func newEngineHTTP(s *Spec, pool *transportPool) *engineHTTP {
	to := time.Duration(s.TimeoutMS) * time.Millisecond
	if to <= 0 {
		to = 4 * time.Second
	}
	e := &engineHTTP{spec: s, pool: pool, timeout: to}
	if s.MinIntervalMS > 0 {
		e.gate = &timeGate{min: time.Duration(s.MinIntervalMS) * time.Millisecond}
	}
	return e
}

// timeGate enforces a minimum spacing between requests to one engine,
// serializing callers (a bounded wait — ctx cancels it).
type timeGate struct {
	mu   sync.Mutex
	min  time.Duration
	last time.Time
}

func (g *timeGate) wait(ctx context.Context) error {
	for {
		g.mu.Lock()
		wait := g.min - time.Since(g.last)
		if wait <= 0 {
			g.last = time.Now()
			g.mu.Unlock()
			return nil
		}
		g.mu.Unlock()
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// fetch renders URL/headers/body for q and performs the request. It maps
// transport-level failures to ErrBlocked (403/429 or a block marker in a
// 200 body) so the fan-out layer can report them distinctly.
func (e *engineHTTP) fetch(ctx context.Context, q Query) ([]Result, error) {
	s := e.spec
	if s.RequiresEnv != "" && strings.TrimSpace(os.Getenv(s.RequiresEnv)) == "" {
		return nil, fmt.Errorf("%w: set %s", ErrNotConfigured, s.RequiresEnv)
	}
	// Pace before the request budget applies: waiting for the gate is not
	// upstream latency, so it must not eat the engine's timeout.
	if e.gate != nil {
		if err := e.gate.wait(ctx); err != nil {
			return nil, fmt.Errorf("pacing wait: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Push domain filters into the query text for engines that understand
	// `site:`, so upstream spends its whole result page on the domains the
	// caller asked for instead of us discarding most of it afterwards. The
	// query is URL-encoded by renderURL, so the operators survive intact.
	if s.SiteOperator {
		q.Q = siteOperatorQuery(q.Q, q)
	}

	rawURL := renderURL(s.Request.URL, q)
	body := renderURL(s.Request.Body, q)
	headers := make(map[string]string, len(s.Request.Headers))
	for k, v := range s.Request.Headers {
		headers[k] = expandEnv(v)
	}
	method := s.Request.Method
	if method == "" {
		method = "GET"
	}

	resp, err := e.pool.Do(ctx, s.Profile, s.Proxy, method, rawURL, body, headers)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("timeout after %s: %w", e.timeout, ctx.Err())
		}
		return nil, err
	}
	if resp.Status == http.StatusTooManyRequests || resp.Status == http.StatusForbidden {
		return nil, fmt.Errorf("%w: HTTP %d", ErrBlocked, resp.Status)
	}
	// Any 2xx is parseable: DuckDuckGo answers rate-limit probes with a
	// body-less 202, which is a legitimate "nothing here" signal — an
	// empty parse beats a transport error in the report.
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.Status)
	}
	lower := strings.ToLower(string(resp.Body))
	for _, marker := range s.BlockMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return nil, fmt.Errorf("%w: page contains %q", ErrBlocked, marker)
		}
	}
	results, err := parse(resp.Body, s.Parse)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].Engine = s.Name
	}
	return results, nil
}

// renderURL substitutes {{query}}/{{pageno}}/{{language}}/{{time_range}}
// placeholders (query is URL-encoded) and expands ${ENV} references.
func renderURL(tpl string, q Query) string {
	if tpl == "" {
		return ""
	}
	pageno := q.Pageno
	if pageno <= 0 {
		pageno = 1
	}
	out := strings.NewReplacer(
		"{{query}}", url.QueryEscape(q.Q),
		"{{pageno}}", fmt.Sprintf("%d", pageno),
		"{{language}}", url.QueryEscape(q.Language),
		"{{time_range}}", url.QueryEscape(q.TimeRange),
	).Replace(tpl)
	return expandEnv(out)
}

func expandEnv(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return os.ExpandEnv(s)
}
