// Package sitemap discovers site URLs from robots.txt/sitemap.xml and
// falls back to one-level link discovery when no sitemap exists.
package sitemap

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
)

// DefaultMaxURLs caps total discovered URLs.
const DefaultMaxURLs = 5000

// maxDepth caps recursive sitemap-index traversal.
const maxDepth = 3

// DiscoveredURL is a single discovered page URL with its origin source.
type DiscoveredURL struct {
	URL    string `json:"url"`
	Source string `json:"source"` // "sitemap" or "link"
}

// client is a shared HTTP client with a sane timeout. It refuses non-public
// destinations: the seed URL and every sitemap/link it discovers are
// attacker-influenced, and sitemap traversal follows them recursively.
var client = safeurl.Client(20 * time.Second)

// Discover returns URLs for a site, preferring sitemap.xml entries and
// falling back to one-level link discovery from the seed page.
func Discover(ctx context.Context, seedURL string, maxURLs int) ([]DiscoveredURL, error) {
	if maxURLs <= 0 {
		maxURLs = DefaultMaxURLs
	}

	base, err := url.Parse(seedURL)
	if err != nil {
		return nil, fmt.Errorf("invalid seed URL %q: %w", seedURL, err)
	}

	// 1. robots.txt → Sitemap: lines (with /sitemap.xml default fallback).
	sitemapURLs := []string{}
	if robots, err := fetch(ctx, resolve(base, "/robots.txt")); err == nil {
		sitemapURLs = parseRobotsSitemaps(robots)
	}
	if len(sitemapURLs) == 0 {
		// Try the conventional location.
		sitemapURLs = []string{resolve(base, "/sitemap.xml")}
	}

	found := []DiscoveredURL{}
	seen := make(map[string]bool)
	for _, sm := range sitemapURLs {
		if len(found) >= maxURLs {
			break
		}
		urls := crawlSitemap(ctx, sm, maxDepth, maxURLs-len(found))
		for _, u := range urls {
			if !seen[u] {
				seen[u] = true
				found = append(found, DiscoveredURL{URL: u, Source: "sitemap"})
			}
		}
		if len(found) >= maxURLs {
			break
		}
	}

	// 2. Fallback: one-level link discovery when the sitemap was empty.
	if len(found) == 0 {
		links := discoverLinks(ctx, seedURL)
		for _, u := range links {
			if !seen[u] {
				seen[u] = true
				found = append(found, DiscoveredURL{URL: u, Source: "link"})
			}
		}
	}

	return found, nil
}

// crawlSitemap recursively walks a sitemap (or sitemap index), returning
// all page URLs up to max.
func crawlSitemap(ctx context.Context, sitemapURL string, depth, max int) []string {
	if depth <= 0 || max <= 0 {
		return nil
	}

	body, err := fetch(ctx, sitemapURL)
	if err != nil {
		return nil
	}

	var index struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
		Sitemaps []struct {
			Loc string `xml:"loc"`
		} `xml:"sitemap"`
	}
	if err := xml.Unmarshal(body, &index); err != nil {
		return nil
	}

	// Sitemap index → recurse into children.
	if len(index.Sitemaps) > 0 {
		out := []string{}
		for _, sm := range index.Sitemaps {
			if len(out) >= max {
				break
			}
			out = append(out, crawlSitemap(ctx, sm.Loc, depth-1, max-len(out))...)
		}
		return out
	}

	out := []string{}
	for _, u := range index.URLs {
		if len(out) >= max {
			break
		}
		loc := strings.TrimSpace(u.Loc)
		if loc != "" {
			out = append(out, loc)
		}
	}
	return out
}

// parseRobotsSitemaps extracts Sitemap: lines from robots.txt content.
func parseRobotsSitemaps(robots []byte) []string {
	out := []string{}
	for _, line := range strings.Split(string(robots), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "sitemap:") {
			u := strings.TrimSpace(line[len("sitemap:"):])
			if u != "" {
				out = append(out, u)
			}
		}
	}
	return out
}

// discoverLinks returns one-level same-domain links from a page.
func discoverLinks(ctx context.Context, pageURL string) []string {
	body, err := fetch(ctx, pageURL)
	if err != nil {
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}

	out := []string{}
	seen := make(map[string]bool)
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" || strings.HasPrefix(href, "#") {
			return
		}
		resolved := base.ResolveReference(mustParse(href))
		if resolved == nil || resolved.Hostname() != base.Hostname() {
			return
		}
		resolved.Fragment = ""
		u := resolved.String()
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	})
	return out
}

// fetch GETs a URL, returning the body (limited to 10MB).
func fetch(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CinderBot/1.0 (+http://github.com/Michael-Obele/tomoshibi)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

// resolve resolves a path against a base URL.
func resolve(base *url.URL, path string) string {
	u := *base
	u.Path = path
	u.RawQuery = ""
	return u.String()
}

// mustParse parses a URL, returning nil on error.
func mustParse(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return u
}
