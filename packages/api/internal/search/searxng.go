package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SearXNGService implements Service against a self-hosted SearXNG
// meta-search instance. SearXNG aggregates many upstream engines (Google,
// Bing, DuckDuckGo, Brave, Mojeek, Wikipedia, ...), so a single instance
// removes the single-point-of-failure of scraping one engine directly and
// costs nothing beyond one container.
//
// There is deliberately NO client-side rate limiter here: SearXNG is
// self-hosted and handles concurrent searches internally (its per-engine
// rate limiting lives in its own settings). A client limiter would serialize
// every query to ~1 req/s and turn concurrent load into a queue — exactly
// what the load benchmark caught before it was removed.
type SearXNGService struct {
	client   *http.Client
	endpoint string
}

// NewSearXNGService creates a service pointing at a SearXNG base URL such as
// "http://localhost:8889". The JSON API must be enabled on the instance
// (search.formats includes json).
func NewSearXNGService(endpoint string) *SearXNGService {
	return &SearXNGService{
		client:   &http.Client{Timeout: 20 * time.Second},
		endpoint: strings.TrimRight(endpoint, "/"),
	}
}

// searxngResult mirrors the JSON shape of a single SearXNG result.
type searxngResult struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Engine        string   `json:"engine"`
	Score         *float64 `json:"score"`
	PublishedDate *string  `json:"publishedDate"`
}

// searxngResponse mirrors the top-level JSON shape of a SearXNG search.
type searxngResponse struct {
	Query           string          `json:"query"`
	NumberOfResults int             `json:"number_of_results"`
	Results         []searxngResult `json:"results"`
}

func (s *SearXNGService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
	if opts.Limit == 0 {
		opts.Limit = 10
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if err := ValidateCategory(opts.Category); err != nil {
		return nil, 0, err
	}

	u, err := url.Parse(s.endpoint + "/search")
	if err != nil {
		return nil, 0, fmt.Errorf("invalid searxng endpoint: %w", err)
	}
	q := u.Query()
	q.Set("q", opts.Query)
	q.Set("format", "json")
	if opts.Offset > 0 {
		// SearXNG paginates by page number; approximate offset as pageno.
		q.Set("pageno", strconv.Itoa(opts.Offset/opts.Limit+1))
	}
	if opts.MaxAge != nil {
		switch *opts.MaxAge {
		case 1:
			q.Set("time_range", "day")
		case 7:
			q.Set("time_range", "week")
		case 30:
			q.Set("time_range", "month")
		}
	}
	// Map explicit Category (general|news|code) to SearXNG categories.
	// Research via Firecrawl docker at http://localhost:3002:
	//  - Exa: category param accepts company, publication, news, people, etc. (docs.exa.ai/reference/search)
	//  - Firecrawl: sources=[web,news,images] and categories=[research,pdf,developer] (docs.firecrawl.dev/features/search + /api-reference/endpoint/search)
	//  - SearXNG: categories query param is comma-separated (general, news, it, ...) per docs.searxng.org/dev/search_api.html
	// Mapping condensed to three buckets: general→general, news→news, code→it.
	if opts.Category != "" {
		switch opts.Category {
		case "general":
			q.Set("categories", "general")
		case "news":
			q.Set("categories", "news")
		case "code":
			q.Set("categories", "it")
		}
	} else if opts.Mode != "" {
		// Legacy Mode mapping retained for backwards compatibility when Category is absent.
		switch opts.Mode {
		case "news":
			q.Set("categories", "news")
		case "code":
			q.Set("categories", "it")
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("searxng request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("searxng status %d", resp.StatusCode)
	}

	var out searxngResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("searxng parse: %w", err)
	}

	results := make([]Result, 0, len(out.Results))
	for i, r := range out.Results {
		if len(results) >= opts.Limit {
			break
		}
		if r.URL == "" || r.Title == "" {
			continue
		}
		relevance := 0.5
		if r.Score != nil {
			relevance = *r.Score
		}
		results = append(results, Result{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Content,
			ID:          fmt.Sprintf("%s_%d", opts.Query, opts.Offset+i),
			Domain:      extractDomain(r.URL),
			Relevance:   relevance,
			Highlights:  extractHighlights(r.Content, opts.Query),
		})
	}

	if opts.Rerank {
		results = rerankTFIDF(opts.Query, results)
	}

	total := out.NumberOfResults
	if total == 0 {
		total = opts.Offset + len(results)
	}
	if len(results) >= opts.Limit {
		// SearXNG does not expose a reliable total; signal more exist.
		total = opts.Offset + len(results) + 1
	}
	return results, total, nil
}

// extractHighlights returns a single query-biased snippet (120-char window) around the first
// query term found in description, mirroring Firecrawl/Exa highlights. Case-insensitive,
// word-boundary aware. Firecrawl research: highlights are query-relevant excerpts, not plain description.
func extractHighlights(description, query string) []string {
	if description == "" || query == "" {
		return nil
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil
	}
	lowerDesc := strings.ToLower(description)
	bestIdx := -1
	var bestTerm string
	for _, t := range terms {
		lt := strings.ToLower(t)
		if idx := strings.Index(lowerDesc, lt); idx != -1 {
			if bestIdx == -1 || idx < bestIdx {
				bestIdx = idx
				bestTerm = t
			}
		}
	}
	if bestIdx == -1 {
		// No term found — fallback to truncated description (120 chars)
		if len(description) > 120 {
			return []string{strings.TrimSpace(description[:120]) + "…"}
		}
		return []string{description}
	}
	// 60 chars before, term, 60 after = ~120 window
	start := bestIdx - 60
	if start < 0 {
		start = 0
	} else {
		// snap to word boundary
		if sp := strings.Index(description[start:bestIdx], " "); sp != -1 && sp < 10 {
			start += sp + 1
		}
	}
	end := bestIdx + len(bestTerm) + 60
	if end > len(description) {
		end = len(description)
	} else {
		if sp := strings.LastIndex(description[bestIdx+len(bestTerm):end], " "); sp != -1 {
			end = bestIdx + len(bestTerm) + sp
		}
	}
	snippet := strings.TrimSpace(description[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(description) {
		snippet += "…"
	}
	if snippet == "" {
		return nil
	}
	return []string{snippet}
}
