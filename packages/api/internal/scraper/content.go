package scraper

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
)

// ReadabilityResult holds main-content extraction output.
type ReadabilityResult struct {
	Title       string
	Excerpt     string
	Byline      string
	SiteName    string
	ContentHTML string
}

// ExtractMainContent runs readability on rawHTML. On failure it returns a
// fallback result containing the original HTML and a nil error — callers
// must never fail a scrape because extraction failed.
func ExtractMainContent(rawHTML string, pageURL string) (*ReadabilityResult, error) {
	parsed, err := readability.FromReader(strings.NewReader(rawHTML), mustParseURL(pageURL))
	if err != nil {
		return &ReadabilityResult{ContentHTML: rawHTML}, nil
	}
	return &ReadabilityResult{
		Title:       parsed.Title,
		Excerpt:     parsed.Excerpt,
		Byline:      parsed.Byline,
		SiteName:    parsed.SiteName,
		ContentHTML: parsed.Content,
	}, nil
}

// mustParseURL parses pageURL, returning a zero URL on error so readability
// still functions for relative-link resolution.
func mustParseURL(pageURL string) *url.URL {
	u, err := url.Parse(pageURL)
	if err != nil {
		return &url.URL{}
	}
	return u
}

// applyReadabilityMetadata copies non-empty readability fields into the
// result metadata map so consumers get title/excerpt/byline/site_name.
func applyReadabilityMetadata(meta map[string]string, rc *ReadabilityResult) {
	if rc == nil {
		return
	}
	setMeta := func(key, val string) {
		if val != "" {
			meta[key] = val
		}
	}
	setMeta("title", rc.Title)
	setMeta("excerpt", rc.Excerpt)
	setMeta("byline", rc.Byline)
	setMeta("site_name", rc.SiteName)
}

// adSelectors matches common ad/tracker containers removed before markdown
// conversion. blockAds defaults to true in service.go; these are best-effort.
var adSelectors = []string{
	".ad", ".ads", ".advert", ".advertisement", ".banner-ad",
	"[class*='advert']", "[id*='advert']", "[class*='ad-banner']",
	"iframe[src*='doubleclick']", "iframe[src*='googlesyndication']",
	"[aria-label='Advertisement']",
}

// cleanContent applies the block_ads and remove_base64_images passes to raw
// HTML, returning HTML ready for markdown conversion. Both default on.
func cleanContent(rawHTML string, blockAds, removeBase64Images bool) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return rawHTML
	}
	if blockAds {
		for _, sel := range adSelectors {
			doc.Find(sel).Remove()
		}
	}
	if removeBase64Images {
		doc.Find("img[src^='data:']").Remove()
	}
	out, err := doc.Html()
	if err != nil {
		return rawHTML
	}
	return out
}
