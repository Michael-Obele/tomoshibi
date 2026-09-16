package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
	"github.com/PuerkitoBio/goquery"
	"github.com/brianvoe/gofakeit/v6"
)

// BrowserFetcher fetches HTML for a URL in a stealth browser tab.
// ChromedpScraper implements this; tests use a stub.
type BrowserFetcher interface {
	FetchHTML(ctx context.Context, url string) (string, error)
}

// StealthService scrapes Brave Search HTML as a fallback when APIs are
// unavailable. It implements search.Service and is intended as the last
// backend in the HybridService chain (SearXNG → Brave API → Stealth).
type StealthService struct {
	fetcher  BrowserFetcher
	endpoint string
	client   *http.Client
}

// NewStealthService creates a StealthService. Endpoint defaults to
// https://search.brave.com when empty.
func NewStealthService(fetcher BrowserFetcher, endpoint string) *StealthService {
	if endpoint == "" {
		endpoint = "https://search.brave.com"
	}
	return &StealthService{
		fetcher:  fetcher,
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   safeurl.Client(20 * time.Second),
	}
}

// Search performs a Brave Search HTML scrape. It validates category, builds
// the Brave Search URL, fetches HTML via the BrowserFetcher (or plain HTTP
// fallback when fetcher is nil), and parses results with goquery.
func (s *StealthService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
	if err := ValidateCategory(opts.Category); err != nil {
		return nil, 0, err
	}
	if opts.Limit == 0 {
		opts.Limit = 10
	}

	u, err := url.Parse(s.endpoint + "/search")
	if err != nil {
		return nil, 0, fmt.Errorf("stealth parse endpoint: %w", err)
	}
	q := u.Query()
	q.Set("q", opts.Query)
	if opts.Offset > 0 {
		q.Set("offset", strconv.Itoa(opts.Offset))
	}
	u.RawQuery = q.Encode()

	var html string
	if s.fetcher != nil {
		fetched, fetchErr := s.fetcher.FetchHTML(ctx, u.String())
		if fetchErr != nil {
			return nil, 0, fmt.Errorf("stealth fetch: %w", fetchErr)
		}
		html = fetched
	} else {
		if err := safeurl.Check(ctx, u.String()); err != nil {
			return nil, 0, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, 0, fmt.Errorf("stealth request: %w", err)
		}
		req.Header.Set("User-Agent", gofakeit.UserAgent())
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, 0, fmt.Errorf("stealth request: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, 0, fmt.Errorf("stealth status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, 0, fmt.Errorf("stealth read: %w", err)
		}
		html = string(b)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, 0, fmt.Errorf("stealth parse: %w", err)
	}

	results := make([]Result, 0, opts.Limit)
	doc.Find("div[data-type='web'] a[href]").Each(func(i int, sel *goquery.Selection) {
		if len(results) >= opts.Limit {
			return
		}
		href, _ := sel.Attr("href")
		title := strings.TrimSpace(sel.Text())
		if href == "" || title == "" {
			return
		}
		results = append(results, Result{
			Title:       title,
			URL:         href,
			Description: "",
			Domain:      extractDomain(href),
			Relevance:   0.5 - float64(i)*0.05,
			ID:          fmt.Sprintf("%s_%d", opts.Query, opts.Offset+i),
		})
	})

	if len(results) == 0 {
		doc.Find("a[href^='http']").Each(func(i int, sel *goquery.Selection) {
			if len(results) >= opts.Limit {
				return
			}
			href, _ := sel.Attr("href")
			title := strings.TrimSpace(sel.Text())
			if href == "" || title == "" || len(title) < 5 {
				return
			}
			if strings.Contains(href, "brave.com") {
				return
			}
			results = append(results, Result{
				Title:     title,
				URL:       href,
				Domain:    extractDomain(href),
				Relevance: 0.5,
				ID:        fmt.Sprintf("%s_%d", opts.Query, opts.Offset+i),
			})
		})
	}

	return results, len(results), nil
}
