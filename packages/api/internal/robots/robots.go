// Package robots implements the parts of RFC 9309 a crawler actually needs:
// group selection, Allow/Disallow matching with wildcards and end anchors,
// and the widely deployed Crawl-delay / Request-rate directives.
//
// It is deliberately separate from internal/sitemap. That package reads
// robots.txt only to find Sitemap: lines; a crawler needs the permission
// rules, and mixing the two would make either change risk the other.
package robots

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxRobotsBytes is RFC 9309's minimum parse size (500 KiB); reading at least
// this much means a large but valid file is not silently truncated.
const maxRobotsBytes = 512 << 10

// DefaultTTL is how long a fetched file is reused. RFC 9309 permits caching
// for up to 24 hours; an hour keeps policy changes visible without refetching
// per page.
const DefaultTTL = time.Hour

// userAgent identifies the crawler when fetching robots.txt. Unlike scrape
// requests, this one should be honest: the file is a crawler talking to a
// crawler.
const userAgent = "Tomoshibi/1.0"

// Rules is the permission set that applies to one crawler token.
type Rules struct {
	rules       []rule
	crawlDelay  time.Duration
	requestRate time.Duration
}

type group struct {
	agents      []string
	rules       []rule
	crawlDelay  time.Duration
	requestRate time.Duration
}

type rule struct {
	allow       bool
	pattern     string
	re          *regexp.Regexp
	specificity int
}

// Parse reads a robots.txt body and returns the rules that apply to agent.
//
// Matching follows RFC 9309: exact token match first, falling back to the "*"
// group only when no specific group exists; all matching groups are merged.
func Parse(body []byte, agent string) *Rules {
	groups := parseGroups(body)
	selected := selectGroups(groups, strings.ToLower(agent))

	out := &Rules{}
	for _, g := range selected {
		out.rules = append(out.rules, g.rules...)
		if g.crawlDelay > out.crawlDelay {
			out.crawlDelay = g.crawlDelay
		}
		if g.requestRate > out.requestRate {
			out.requestRate = g.requestRate
		}
	}
	return out
}

func parseGroups(body []byte) []group {
	var groups []group
	var current *group
	lastWasAgent := false

	for _, raw := range strings.Split(string(body), "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			lastWasAgent = false
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch key {
		case "user-agent":
			if current == nil || !lastWasAgent {
				groups = append(groups, group{})
				current = &groups[len(groups)-1]
			}
			current.agents = append(current.agents, strings.ToLower(value))
			lastWasAgent = true
			continue
		case "allow", "disallow":
			lastWasAgent = false
			if current == nil || value == "" {
				// An empty Disallow means "allow everything", which is the
				// absence of a rule rather than a rule that allows all.
				continue
			}
			re, specificity := compile(value)
			current.rules = append(current.rules, rule{
				allow:       key == "allow",
				pattern:     value,
				re:          re,
				specificity: specificity,
			})
			continue
		case "crawl-delay":
			lastWasAgent = false
			if current != nil {
				if d := parseSeconds(value); d > current.crawlDelay {
					current.crawlDelay = d
				}
			}
			continue
		case "request-rate":
			lastWasAgent = false
			if current != nil {
				if d := parseRequestRate(value); d > current.requestRate {
					current.requestRate = d
				}
			}
			continue
		default:
			lastWasAgent = false
		}
	}
	return groups
}

// selectGroups implements the RFC's precedence: if any group names the token
// exactly, only those groups apply; otherwise the "*" groups do. The two are
// never merged.
func selectGroups(groups []group, agent string) []group {
	var exact, wildcard []group
	for _, g := range groups {
		isExact, isWildcard := false, false
		for _, a := range g.agents {
			if a == agent {
				isExact = true
			}
			if a == "*" {
				isWildcard = true
			}
		}
		if isExact {
			exact = append(exact, g)
		} else if isWildcard {
			wildcard = append(wildcard, g)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return wildcard
}

// Allowed reports whether path (path plus query, as sent on the wire) may be
// fetched. The longest matching pattern wins; an equal-length tie goes to
// Allow, per RFC 9309.
func (r *Rules) Allowed(path string) bool {
	if path == "" {
		path = "/"
	}
	best := -1
	allowed := true
	for _, rule := range r.rules {
		if !rule.re.MatchString(path) {
			continue
		}
		if rule.specificity > best || (rule.specificity == best && rule.allow) {
			best = rule.specificity
			allowed = rule.allow
		}
	}
	return allowed
}

// CrawlDelay is the Crawl-delay directive, or zero when absent.
func (r *Rules) CrawlDelay() time.Duration { return r.crawlDelay }

// RequestRate is the interval implied by Request-rate, or zero when absent.
func (r *Rules) RequestRate() time.Duration { return r.requestRate }

// Delay is the strictest spacing the file asks for.
func (r *Rules) Delay() time.Duration {
	if r.requestRate > r.crawlDelay {
		return r.requestRate
	}
	return r.crawlDelay
}

// compile turns a robots pattern into an anchored regexp. "*" matches any
// sequence (including "/"), a trailing "$" anchors the end, and everything
// else is literal.
func compile(pattern string) (*regexp.Regexp, int) {
	var b strings.Builder
	b.WriteString("^")
	specificity := 0
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			b.WriteString(".*")
		case '$':
			if i == len(pattern)-1 {
				b.WriteString("$")
			} else {
				b.WriteString(`\$`)
				specificity++
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			specificity++
		}
	}
	return regexp.MustCompile(b.String()), specificity
}

// parseSeconds accepts the integer and decimal forms sites use.
func parseSeconds(value string) time.Duration {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

// parseRequestRate reads "N/interval" (for example "1/5" or "1/1m") and
// returns the spacing between requests. The unit defaults to seconds.
func parseRequestRate(value string) time.Duration {
	countRaw, intervalRaw, ok := strings.Cut(value, "/")
	if !ok {
		return 0
	}
	count, err := strconv.ParseFloat(strings.TrimSpace(countRaw), 64)
	if err != nil || count <= 0 {
		return 0
	}
	intervalRaw = strings.TrimSpace(intervalRaw)
	unit := time.Second
	switch {
	case strings.HasSuffix(intervalRaw, "ms"):
		unit = time.Millisecond
		intervalRaw = strings.TrimSuffix(intervalRaw, "ms")
	case strings.HasSuffix(intervalRaw, "s"):
		intervalRaw = strings.TrimSuffix(intervalRaw, "s")
	case strings.HasSuffix(intervalRaw, "m"):
		unit = time.Minute
		intervalRaw = strings.TrimSuffix(intervalRaw, "m")
	case strings.HasSuffix(intervalRaw, "h"):
		unit = time.Hour
		intervalRaw = strings.TrimSuffix(intervalRaw, "h")
	}
	interval, err := strconv.ParseFloat(strings.TrimSpace(intervalRaw), 64)
	if err != nil || interval <= 0 {
		return 0
	}
	return time.Duration(interval / count * float64(unit))
}

// Cache fetches and reuses robots.txt per origin. A 4xx answer is cached as
// "no restrictions"; a 5xx or transport failure is not cached, so a transient
// outage does not pin a site as unrestricted for the whole TTL.
type Cache struct {
	client *http.Client
	ttl    time.Duration

	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	rules   *Rules
	fetched time.Time
}

// NewCache builds a cache. A nil client uses a bounded default.
func NewCache(client *http.Client) *Cache {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Cache{
		client:  client,
		ttl:     DefaultTTL,
		entries: make(map[string]*entry),
	}
}

// For returns the rules for the origin of rawURL, fetching robots.txt on a
// cache miss.
func (c *Cache) For(ctx context.Context, rawURL string) (*Rules, error) {
	origin, err := originOf(rawURL)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if e, ok := c.entries[origin]; ok && time.Since(e.fetched) < c.ttl {
		c.mu.Unlock()
		return e.rules, nil
	}
	c.mu.Unlock()

	rules, err := c.fetch(ctx, origin)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[origin] = &entry{rules: rules, fetched: time.Now()}
	c.mu.Unlock()
	return rules, nil
}

func (c *Cache) fetch(ctx context.Context, origin string) (*Rules, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/robots.txt", nil)
	if err != nil {
		return nil, fmt.Errorf("build robots request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch robots.txt: %w", err)
	}
	defer resp.Body.Close()

	// 4xx means "no robots file": the whole origin is allowed.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return &Rules{}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("robots.txt returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRobotsBytes))
	if err != nil {
		return nil, fmt.Errorf("read robots.txt: %w", err)
	}
	return Parse(body, userAgentToken()), nil
}

// userAgentToken is the product token the rules are matched against.
func userAgentToken() string {
	token, _, _ := strings.Cut(userAgent, "/")
	return strings.ToLower(token)
}

// originOf returns scheme://host[:port] with default ports omitted, which is
// the scope RFC 9309 defines for a robots.txt file.
func originOf(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("URL has no host")
	}

	host := u.Host
	if (u.Scheme == "http" && u.Port() == "80") || (u.Scheme == "https" && u.Port() == "443") {
		host = u.Hostname()
	}
	return u.Scheme + "://" + host, nil
}
