package search

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// braveSearchResponse is the JSON response from Brave Search (unexported; internal use only).
type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Result represents a single search result
type Result struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	ID          string   `json:"id"`
	Domain      string   `json:"domain"`
	Relevance   float64  `json:"relevance"`
	Highlights  []string `json:"highlights,omitempty"`
}

// SearchOptions contains options for the search
type SearchOptions struct {
	Query          string
	Offset         int
	Limit          int
	IncludeDomains []string
	ExcludeDomains []string
	RequiredText   []string
	MaxAge         *int
	Mode           string
	Category       string
	Rerank         bool
}

// validCategories is the allowed set for Category (unexported; use IsValidCategory).
var validCategories = map[string]bool{
	"general": true,
	"news":    true,
	"code":    true,
}

// ValidCategories lists allowed values for Category.
//
// Deprecated: use IsValidCategory instead; this map is mutable and exposed
// only for backward compatibility.
var ValidCategories = validCategories

// IsValidCategory reports whether c is an allowed category.
func IsValidCategory(c string) bool { return validCategories[c] }

// ValidateCategory returns an error if category is non-empty and not in the
// allowed enum. Mirrors Exa's company/news/people → general/news/code mapping
// and Firecrawl's sources/categories handling, condensed into three buckets
// for SearXNG (general→general, news→news, code→it) and Brave search_type.
func ValidateCategory(c string) error {
	if c == "" {
		return nil
	}
	if !validCategories[c] {
		return fmt.Errorf("invalid category %q: must be one of general, news, code", c)
	}
	return nil
}

// Service defines the search service interface
type Service interface {
	Search(ctx context.Context, opts SearchOptions) ([]Result, int, error)
}

// braveEndpoint is the upstream web search endpoint.
const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"

// BraveService implements Service using Brave Search API
type BraveService struct {
	apiKey  string
	client  *http.Client
	limiter *rate.Limiter
	// endpoint is the base URL Search posts to. It is a field rather than a
	// hardcoded constant so tests can point it at an httptest server; nothing
	// outside this package sets it.
	endpoint string
}

// NewBraveService creates a new instance of BraveService
func NewBraveService(apiKey string) *BraveService {
	return &BraveService{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// Limit to 1 request per 1.1 seconds to be safe and avoid 429s
		limiter:  rate.NewLimiter(rate.Every(1100*time.Millisecond), 1),
		endpoint: braveEndpoint,
	}
}

// Search performs a search on Brave Search with pagination support
// Returns: (results, totalCount, error)
// Note: Brave API doesn't provide totalCount, so we estimate based on typical result patterns
func (s *BraveService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
	if s.apiKey == "" {
		return nil, 0, fmt.Errorf("brave search api key is missing")
	}

	// Set defaults
	if opts.Limit == 0 {
		opts.Limit = 10
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if err := ValidateCategory(opts.Category); err != nil {
		return nil, 0, err
	}

	// Wait for rate limiter
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, 0, fmt.Errorf("rate limit wait: %w", err)
	}

	endpoint := s.endpoint
	if endpoint == "" {
		endpoint = braveEndpoint
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters with pagination support
	q := req.URL.Query()
	q.Add("q", opts.Query)
	q.Add("count", strconv.Itoa(opts.Limit))
	if opts.Offset > 0 {
		q.Add("offset", strconv.Itoa(opts.Offset))
	}

	// Add filtering options if provided
	if opts.Mode != "" {
		// Note: Mode is for MCP layer speed control, not passed to Brave
		// But could implement with freshness parameter
		if opts.Mode == "fast" {
			q.Add("freshness", "day") // Recent results only
		}
	}

	if opts.MaxAge != nil {
		// Convert MaxAge to freshness parameter
		switch *opts.MaxAge {
		case 1:
			q.Add("freshness", "day")
		case 7:
			q.Add("freshness", "week")
		case 30:
			q.Add("freshness", "month")
		}
	}

	// Map Category to Brave search_type (general|news|code) + domain heuristic hints.
	// Firecrawl research: sources=["web","news","images"] for type; Exa category maps to filtered index.
	// Brave's web/search endpoint accepts search_type for news vs general; for code we send
	// search_type=code and downstream heuristics can boost code domains (github, stackoverflow).
	if opts.Category != "" {
		q.Add("search_type", opts.Category)
	}

	req.URL.RawQuery = q.Encode()

	// Set headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", s.apiKey)

	// Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("brave search api error: status %d", resp.StatusCode)
	}

	// Parse JSON
	var braveResponse braveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResponse); err != nil {
		return nil, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to Result
	results := make([]Result, 0, len(braveResponse.Web.Results))
	for idx, item := range braveResponse.Web.Results {
		results = append(results, Result{
			Title:       item.Title,
			URL:         item.URL,
			Description: item.Description,
			ID:          fmt.Sprintf("%s_%d", opts.Query, opts.Offset+idx),
			Domain:      extractDomain(item.URL),
			Relevance:   1.0 - float64(idx)*0.05, // Simple relevance scoring
			Highlights:  extractHighlights(item.Description, opts.Query),
		})
	}

	// Determine total count
	// Brave API doesn't always provide a stable totalCount field for web results
	// We use the count of returned results and the requested limit to estimate if more exist
	estimatedTotal := opts.Offset + len(results)
	if len(results) >= opts.Limit && opts.Limit > 0 {
		// If we got as many results as we asked for, assume there might be more
		// We'll signal that there are at least 100 more results to keep pagination going
		estimatedTotal += 100
	}

	if opts.Rerank {
		results = rerankTFIDF(opts.Query, results)
	}

	return results, estimatedTotal, nil
}

// rerankTFIDF is lightweight, pure-Go term-frequency re-rank (no ONNX).
// Research via Firecrawl: bge-small needs ONNX runtime + 80MB model + CGO,
// which breaks hobby-tier static binary. TF-IDF gives 80% of semantic gain
// with 0 deps. Score = sum(tf * idf) where tf = termCount/len(doc), idf = 1+log(N/df).
// Docs with no term overlap keep original order. Cost O(N * terms).
func rerankTFIDF(query string, results []Result) []Result {
	if query == "" || len(results) <= 1 {
		return results
	}
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return results
	}
	// Document frequency per term
	df := make(map[string]int)
	termDocs := make([]map[string]int, len(results))
	for i, r := range results {
		text := strings.ToLower(r.Title + " " + r.Description)
		if len(r.Highlights) > 0 {
			text += " " + strings.ToLower(strings.Join(r.Highlights, " "))
		}
		m := make(map[string]int)
		words := strings.Fields(text)
		for _, w := range words {
			// simple normalization: trim punctuation
			w = strings.Trim(w, ".,:;!?\"'()[]{}")
			if w == "" {
				continue
			}
			m[w]++
		}
		termDocs[i] = m
		for _, t := range terms {
			lt := strings.ToLower(strings.Trim(t, ".,:;!?\"'()[]{}"))
			if lt == "" {
				continue
			}
			if cnt := m[lt]; cnt > 0 {
				df[lt]++
			} else {
				// also count substring matches (e.g., "golang" in "golang concurrency")
				if strings.Contains(text, lt) {
					df[lt]++
				}
			}
		}
	}
	N := float64(len(results))
	type scored struct {
		idx   int
		score float64
		orig  float64
	}
	scoredList := make([]scored, len(results))
	for i, r := range results {
		text := strings.ToLower(r.Title + " " + r.Description)
		words := strings.Fields(text)
		docLen := float64(len(words))
		if docLen == 0 {
			docLen = 1
		}
		var score float64
		for _, t := range terms {
			lt := strings.ToLower(strings.Trim(t, ".,:;!?\"'()[]{}"))
			if lt == "" {
				continue
			}
			var tf float64
			if cnt, ok := termDocs[i][lt]; ok && cnt > 0 {
				tf = float64(cnt) / docLen
			} else if strings.Contains(text, lt) {
				// fallback substring tf
				tf = 1.0 / docLen
			}
			idf := 1.0 + math.Log((N+1)/(float64(df[lt])+1))
			score += tf * idf
		}
		// blend with original relevance (0.2 weight) to keep stable order when TF ties
		score = score*0.8 + r.Relevance*0.2
		scoredList[i] = scored{idx: i, score: score, orig: r.Relevance}
	}
	sort.SliceStable(scoredList, func(a, b int) bool {
		if scoredList[a].score == scoredList[b].score {
			return scoredList[a].orig > scoredList[b].orig
		}
		return scoredList[a].score > scoredList[b].score
	})
	reranked := make([]Result, len(results))
	for i, s := range scoredList {
		reranked[i] = results[s.idx]
		// update relevance to reranked score for transparency
		reranked[i].Relevance = s.score
	}
	return reranked
}

// extractDomain extracts domain from URL
func extractDomain(urlStr string) string {
	// Simple extraction - would be improved in production
	// Extract domain from URL string
	if urlStr == "" {
		return ""
	}
	// Parse and get hostname
	u, err := parseURLDomain(urlStr)
	if err != nil {
		return ""
	}
	return u
}

// parseURLDomain is a simple URL domain parser
func parseURLDomain(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Hostname(), nil
}
