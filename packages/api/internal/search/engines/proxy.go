package engines

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProxyPool resolves proxy groups to proxy URLs and rotates through them.
//
// Groups (config-over-hardcoded, all optional):
//   - webshare: WEBSHARE_PROXY_URL (explicit endpoint(s)) or WEBSHARE_API_KEY
//     (fetches the account's proxy list from the Webshare API once and
//     round-robins it). Webshare's free tier is datacenter class — plumbing
//     and rotation testing, not a Google-grade bypass (research.md §5).
//   - tier1 / tier2: SEARCH_PROXY_TIER1 / SEARCH_PROXY_TIER2, comma-separated.
//
// An unconfigured group returns "" → the engine goes direct (degrades, never
// fails because a proxy is missing).
type ProxyPool struct {
	mu     sync.Mutex
	groups map[string]*proxyGroup
	// webshareKey defers the Webshare API fetch until first use so a
	// missing key costs nothing and startup never blocks on the vendor API.
	webshareKey  string
	webshareOnce sync.Once
	budget       *budgetMeter

	// webshareAPIBase is a field so tests can point at a stub server.
	webshareAPIBase string
}

type proxyGroup struct {
	urls []string
	rr   int
}

// ProxyOptions is the env-derived configuration (see config.SearchConfig).
type ProxyOptions struct {
	WebshareAPIKey   string
	WebshareProxyURL string
	Tier1            string
	Tier2            string
	BudgetGB         string
}

// NewProxyPool builds groups from options. It performs no network I/O.
func NewProxyPool(opts ProxyOptions) *ProxyPool {
	p := &ProxyPool{
		groups:          map[string]*proxyGroup{},
		budget:          newBudgetMeter(opts.BudgetGB),
		webshareAPIBase: "https://proxy.webshare.io",
	}
	addGroup := func(name, raw string) {
		urls := splitCSV(raw)
		if len(urls) > 0 {
			p.groups[name] = &proxyGroup{urls: urls}
		}
	}
	// Explicit endpoint wins over API discovery (a backbone -rotate URL is
	// a single self-rotating endpoint).
	addGroup("webshare", opts.WebshareProxyURL)
	g, _ := p.groups["webshare"]
	if (g == nil || len(g.urls) == 0) && strings.TrimSpace(opts.WebshareAPIKey) != "" {
		if g == nil {
			g = &proxyGroup{}
			p.groups["webshare"] = g
		}
		p.webshareKey = opts.WebshareAPIKey
	}
	addGroup("tier1", opts.Tier1)
	addGroup("tier2", opts.Tier2)
	return p
}

// Next returns the next proxy URL for a group (round-robin), or "" when the
// group is unconfigured / the budget is exhausted.
func (p *ProxyPool) Next(group string) string {
	if group == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	g, ok := p.groups[group]
	if !ok {
		return ""
	}
	if group == "webshare" && len(g.urls) == 0 && p.webshareKey != "" {
		p.webshareOnce.Do(func() {
			urls, err := p.fetchWebshareList(p.webshareKey)
			if err != nil {
				return // stay empty → direct egress
			}
			g.urls = urls
		})
	}
	if len(g.urls) == 0 {
		return ""
	}
	if p.budget != nil && !p.budget.allow() {
		return "" // exhausted: degrade to direct
	}
	u := g.urls[g.rr%len(g.urls)]
	g.rr++
	return u
}

// Spend charges n response bytes against the proxy budget.
func (p *ProxyPool) Spend(n int) {
	if p.budget != nil {
		p.budget.add(int64(n))
	}
}

// BudgetExhausted reports whether the monthly proxy byte cap was hit
// (surfaced in logs so degradation is never silent).
func (p *ProxyPool) BudgetExhausted() bool {
	return p.budget != nil && p.budget.exhausted()
}

// fetchWebshareList pulls the account proxy list, following pagination.
// Docs (apidocs.webshare.io/proxy-list/list):
//
//	GET {base}/api/v2/proxy/list/?mode=direct&page=N&page_size=100
//	Authorization: Token <key>
//	→ {count, next, previous, results:[{username,password,proxy_address,port,valid,…}]}
//
// `next` is null on the last page; accounts bigger than one page are
// followed (bounded) so round-robin covers the whole pool.
func (p *ProxyPool) fetchWebshareList(key string) ([]string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	var urls []string
	next := p.webshareAPIBase + "/api/v2/proxy/list/?mode=direct&page=1&page_size=100"
	for page := 1; next != "" && page <= 10; page++ {
		req, err := http.NewRequest(http.MethodGet, next, nil)
		if err != nil {
			return nil, fmt.Errorf("build webshare request: %w", err)
		}
		req.Header.Set("Authorization", "Token "+key)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("webshare list: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("webshare list: HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("webshare list read: %w", err)
		}
		var payload struct {
			Next    string `json:"next"`
			Results []struct {
				Username     string `json:"username"`
				Password     string `json:"password"`
				ProxyAddress string `json:"proxy_address"`
				Port         int    `json:"port"`
				Valid        bool   `json:"valid"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("webshare list parse: %w", err)
		}
		for _, r := range payload.Results {
			if !r.Valid || r.ProxyAddress == "" || r.Port == 0 {
				continue
			}
			u := fmt.Sprintf("http://%s:%d", r.ProxyAddress, r.Port)
			if r.Username != "" && r.Password != "" {
				u = fmt.Sprintf("http://%s:%s@%s:%d",
					strings.TrimSpace(r.Username), strings.TrimSpace(r.Password), r.ProxyAddress, r.Port)
			}
			urls = append(urls, u)
		}
		// json null → ""; tolerate absolute or root-relative next links.
		next = ""
		switch {
		case strings.HasPrefix(payload.Next, "http"):
			next = payload.Next
		case strings.HasPrefix(payload.Next, "/"):
			next = p.webshareAPIBase + payload.Next
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("webshare list: no valid proxies")
	}
	return urls, nil
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// budgetMeter is a per-process byte cap (plan R3). Monthly persistence is
// deliberately out of scope: a restart resets the counter, which is the
// safe direction (counting only goes down).
type budgetMeter struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

func newBudgetMeter(gb string) *budgetMeter {
	gb = strings.TrimSpace(gb)
	if gb == "" {
		gb = "1" // plan default: SEARCH_PROXY_BUDGET_GB=1
	}
	f, err := strconv.ParseFloat(gb, 64)
	if err != nil || f <= 0 {
		return nil // 0 / invalid = unlimited (explicit opt-out)
	}
	return &budgetMeter{limit: int64(f * (1 << 30))}
}

func (b *budgetMeter) add(n int64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.used += n
	b.mu.Unlock()
}

func (b *budgetMeter) allow() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used < b.limit
}

func (b *budgetMeter) exhausted() bool { return b != nil && !b.allow() }
