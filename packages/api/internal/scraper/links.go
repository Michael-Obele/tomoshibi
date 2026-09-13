package scraper

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// ExtractLinks parses the readability ContentHTML (or any HTML fragment) and
// returns deduped absolute hyperlinks with anchor text and a same-host flag.
//
// Research via Firecrawl docker (http://localhost:3002):
//
//	POST /v1/scrape {"url":"https://example.com","formats":["markdown","links"]}
//	→ {"data":{"links":["https://iana.org/domains/example"]}}  (array of strings)
//
// Cinder enriches each entry to {url, text, isInternal} as required by
// ticket 06-scrape-links-parity. Extraction runs after readability so nav/ads
// are stripped before link collection. Results are cached via ScrapeOptions.
func ExtractLinks(readabilityHTML, pageURL string) []domain.LinkData {
	if strings.TrimSpace(readabilityHTML) == "" {
		return nil
	}
	base, err := url.Parse(pageURL)
	if err != nil || base.Host == "" {
		// Without a valid base we cannot resolve relatives or flag internals.
		// Still try to extract absolute http(s) links.
		base = nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(readabilityHTML))
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	links := []domain.LinkData{}

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		raw, exists := s.Attr("href")
		if !exists {
			return
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		lower := strings.ToLower(raw)
		// Skip fragment-only, javascript, mailto, tel, data URIs.
		if strings.HasPrefix(lower, "javascript:") ||
			strings.HasPrefix(lower, "mailto:") ||
			strings.HasPrefix(lower, "tel:") ||
			strings.HasPrefix(lower, "data:") ||
			strings.HasPrefix(lower, "#") {
			return
		}

		abs := resolveLinkURL(raw, pageURL, base)
		if abs == "" {
			return
		}
		// Only http(s) links
		if !strings.HasPrefix(abs, "http://") && !strings.HasPrefix(abs, "https://") {
			return
		}
		if _, dup := seen[abs]; dup {
			return
		}
		seen[abs] = struct{}{}

		text := strings.TrimSpace(s.Text())
		// Collapse whitespace in anchor text
		if text != "" {
			text = strings.Join(strings.Fields(text), " ")
		}

		var isInternal bool
		if base != nil {
			if u, err := url.Parse(abs); err == nil {
				isInternal = strings.EqualFold(u.Host, base.Host)
			}
		}

		links = append(links, domain.LinkData{
			URL:        abs,
			Text:       text,
			IsInternal: isInternal,
		})
	})

	if len(links) == 0 {
		return nil
	}
	logger.Log.Debug("Extracted links", "page_url", pageURL, "count", len(links))
	return links
}

// resolveLinkURL resolves raw href against pageURL, returning an absolute URL string.
func resolveLinkURL(raw, pageURL string, base *url.URL) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	if base == nil {
		b, err := url.Parse(pageURL)
		if err != nil {
			return ""
		}
		base = b
	}
	// Handle protocol-relative URLs like //example.com/path
	if strings.HasPrefix(raw, "//") {
		scheme := base.Scheme
		if scheme == "" {
			scheme = "https"
		}
		abs := scheme + ":" + raw
		if u, err := url.Parse(abs); err == nil {
			return u.String()
		}
		return abs
	}
	return base.ResolveReference(parsed).String()
}
