package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/search"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type SearchRequest struct {
	Query          string   `json:"query" binding:"required"`
	Offset         int      `json:"offset"`
	Limit          int      `json:"limit"`
	IncludeDomains []string `json:"includeDomains,omitempty"`
	ExcludeDomains []string `json:"excludeDomains,omitempty"`
	RequiredText   []string `json:"requiredText,omitempty"`
	MaxAge         *int     `json:"maxAge,omitempty"`
	Mode           string   `json:"mode"`
	Category       string   `json:"category"`
	Rerank         bool     `json:"rerank"`
	// ScrapeContent fetches each result's page body in the same call and
	// returns it as `content`, instead of the caller making a second request
	// per result. Opt-in: it costs one fetch per result.
	ScrapeContent bool `json:"scrapeContent,omitempty"`
	// ScrapeLimit caps how many top results are fetched when
	// ScrapeContent is set. 0 = every returned result. Ignored otherwise.
	ScrapeLimit int `json:"scrapeLimit,omitempty"`
	// Engine picks a backend explicitly. `browser:ddg`, `browser:google` and
	// `browser:bing` read that engine's results page in your own signed-in
	// browser, which is the way the engines that block a datacenter address still
	// answer. Anything else uses the configured chain.
	Engine string `json:"engine,omitempty"`
}

type SearchResponse struct {
	Query      string          `json:"query"`
	Results    []search.Result `json:"results"`
	HasMore    bool            `json:"hasMore"`
	NextOffset int             `json:"nextOffset"`
	Count      int             `json:"count"`
}

type SearchHandler struct {
	service search.Service
	// scraper is optional. When nil, a scrapeContent request is honoured
	// structurally (the field is accepted) but no content is fetched.
	scraper search.ContentFetcher
	// browser is the browser-backed engine rung. When present, an explicit
	// `engine: browser:<name>` bypasses the chain so the caller gets exactly the
	// engine it named (and its own cache key).
	browser search.Service
	// pages fetches full scrape results for research composition.
	pages PageFetcher
}

// HandlerOption configures optional SearchHandler dependencies.
type HandlerOption func(*SearchHandler)

// WithScraper wires the page fetcher used for search-time content
// composition. Without it, scrapeContent has nothing to fetch through.
func WithScraper(s search.ContentFetcher) HandlerOption {
	return func(h *SearchHandler) { h.scraper = s }
}

// WithBrowserSearch wires the browser-backed engine rung, so `engine:
// browser:google` and friends can be selected per request.
func WithBrowserSearch(s search.Service) HandlerOption {
	return func(h *SearchHandler) { h.browser = s }
}

func NewSearchHandler(s search.Service, opts ...HandlerOption) *SearchHandler {
	h := &SearchHandler{service: s}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Search godoc
// @Summary      Search the web
// @Description  Searches the web using DuckDuckGo HTML (free, no key) with Brave Search as fallback when BRAVE_SEARCH_API_KEY is set.
// @Tags         search
// @Accept       json
// @Produce      json
// @Param        query          query     string  false  "The search query"
// @Param        q              query     string  false  "Alias for query"
// @Param        offset         query     int     false  "Pagination offset"
// @Param        limit          query     int     false  "Pagination limit (max 100)"
// @Param        category       query     string  false  "Category filter: general, news, code" Enums(general,news,code)
// @Param        rerank         query     bool    false  "Lightweight TF-IDF re-rank (pure Go, no ONNX)"
// @Param        scrapeContent  query     bool    false  "Fetch each result's page body and return it as `content`"
// @Param        body           body      SearchRequest  false  "JSON request body"
// @Success      200    {object}  SearchResponse
// @Failure      400    {object}  map[string]interface{}
// @Failure      500    {object}  map[string]interface{}
// @Router       /search [post]
// @Router       /search [get]
// search routes one request: an explicit browser engine goes straight to that
// rung, so the caller gets exactly the engine it named and its own cache key.
// Everything else walks the configured chain.
func (h *SearchHandler) search(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
	if opts.BrowserEngine == "" {
		return h.service.Search(ctx, opts)
	}
	if h.browser == nil {
		return nil, 0, fmt.Errorf(
			"engine browser:%s is not available: pair the bridge extension and allow its host with `tomoshibi bridge allow`",
			opts.BrowserEngine,
		)
	}
	return h.browser.Search(ctx, opts)
}

// resolveEngine parses the `engine` param. Only `browser:<name>` is accepted by
// name: a bare `google` is ambiguous between the native registry entry and the
// browser rung, and guessing is how a caller ends up on a different backend than
// it asked for.
func resolveEngine(body, query string) (string, error) {
	raw := body
	if raw == "" {
		raw = query
	}
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" || v == "auto" {
		return "", nil
	}
	name, ok := strings.CutPrefix(v, "browser:")
	if !ok {
		return "", fmt.Errorf(
			"unknown engine %q: use browser:ddg, browser:google or browser:bing, or omit it for the configured chain",
			raw,
		)
	}
	switch name {
	case "ddg", "google", "bing":
		return name, nil
	}
	return "", fmt.Errorf("unknown browser engine %q: use ddg, google or bing", name)
}

// PageFetcher fetches one page for research composition. *scraper.Service
// satisfies it; the interface keeps the handler testable without a browser.
type PageFetcher interface {
	Scrape(ctx context.Context, url, mode string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error)
}

// WithPageFetcher wires full scrape results, which research needs (provenance,
// page tree, motion) and the plain ContentFetcher seam does not carry.
func WithPageFetcher(f PageFetcher) HandlerOption {
	return func(h *SearchHandler) { h.pages = f }
}

// ResearchRequest is one query turned into read pages.
type ResearchRequest struct {
	Query string `json:"query"`
	// Limit is how many SERP results to ask for. Default 10.
	Limit int `json:"limit"`
	// Pages is how many of those results to actually read. Defaults to min(5,
	// Limit) — a search is cheap, a page read is not.
	Pages int `json:"pages"`
	// Engine picks the search backend: `browser:ddg|google|bing`, or empty for
	// the configured chain.
	Engine string `json:"engine"`
	// Transport picks how pages are read: "" / "anonymous" / "bridge" / "auto".
	// Default "auto": try the cheap lanes first, escalate into your browser only
	// for an allowed origin.
	Transport string `json:"transport"`
	Mode      string `json:"mode"`
	// PageOptions applied to every page in the fan-out.
	Summary       bool   `json:"summary"`
	PageTree      string `json:"page_tree"`
	Motion        bool   `json:"motion"`
	ScrollCollect bool   `json:"scroll_collect"`
}

// ResearchPage is one result of the fan-out: the page, or why it could not be
// read. A failed page never fails the run — a research answer with gaps is still
// an answer, and the gaps are visible here.
type ResearchPage struct {
	URL        string               `json:"url"`
	Title      string               `json:"title,omitempty"`
	Markdown   string               `json:"markdown,omitempty"`
	Summary    string               `json:"summary,omitempty"`
	WordCount  int                  `json:"word_count,omitempty"`
	Provenance *domain.Provenance   `json:"provenance,omitempty"`
	PageTree   *domain.PageNode     `json:"page_tree,omitempty"`
	Motion     *domain.MotionReport `json:"motion,omitempty"`
	Error      string               `json:"error,omitempty"`
}

// ResearchResponse is one query, its SERP, and the pages behind it.
type ResearchResponse struct {
	Query     string          `json:"query"`
	Results   []search.Result `json:"results"`
	Pages     []ResearchPage  `json:"pages"`
	Failures  []ResearchPage  `json:"failures"`
	ElapsedMS int64           `json:"elapsed_ms"`
	// PaidAPIUsed is true when the paid search backend answered, so a caller can
	// see what this query actually cost.
	PaidAPIUsed bool `json:"paid_api_used"`
}

// maxResearchPages caps one fan-out. It is a ceiling on the caller, not a
// suggestion: the borrow lane spends a real session, and a loop with a bug
// should be refused here rather than at the browser budget an hour later.
const maxResearchPages = 20

// Research runs a query through search and then reads the top results, in one
// call, with one trace and one budget.
func (h *SearchHandler) Research(c *gin.Context) {
	var req ResearchRequest
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
			return
		}
	}
	if req.Query == "" {
		req.Query = c.Query("query")
	}
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Pages <= 0 {
		req.Pages = min(5, req.Limit)
	}
	if req.Pages > maxResearchPages {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("pages is capped at %d", maxResearchPages)})
		return
	}
	if h.pages == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "research needs the scrape service, which is not wired"})
		return
	}
	engine, engineErr := resolveEngine(req.Engine, c.Query("engine"))
	if engineErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": engineErr.Error()})
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = "smart"
	}
	transport := req.Transport
	if transport == "" {
		transport = domain.TransportAuto
	}

	ctx := c.Request.Context()
	started := time.Now()

	results, _, err := h.search(ctx, search.SearchOptions{
		Query: req.Query, Limit: req.Limit, BrowserEngine: engine,
	})
	if err != nil {
		// The same mapper as /v1/search, so a challenge carries the same branchable
		// code on both endpoints. A client that reads `code` must not have to know
		// which of the two it called.
		searchErrorResponse(c, err)
		return
	}

	n := min(req.Pages, len(results))
	pages := make([]ResearchPage, n)
	g, gctx := errgroup.WithContext(ctx)
	// Three at a time: the bridge serialises runs anyway, and a larger fan-out
	// only makes the anonymous lanes noisier.
	g.SetLimit(3)
	for i := 0; i < n; i++ {
		i := i
		g.Go(func() error {
			opts := domain.ScrapeOptions{
				Transport:   transport,
				AllowBorrow: true,
				Summary:     req.Summary,
				PageTree:    req.PageTree,
				Motion:      req.Motion,
			}
			if req.ScrollCollect {
				opts.ScrollCollect = &domain.ScrollCollectOptions{}
			}
			res, serr := h.pages.Scrape(gctx, results[i].URL, mode, opts)
			if serr != nil {
				pages[i] = ResearchPage{URL: results[i].URL, Error: serr.Error()}
				return nil
			}
			title := extractTitle(*res)
			pages[i] = ResearchPage{
				URL:        res.URL,
				Title:      title,
				Markdown:   res.Markdown,
				Summary:    res.Summary,
				WordCount:  wordCount(res.Markdown),
				Provenance: res.Provenance,
				PageTree:   res.PageTree,
				Motion:     res.Motion,
			}
			return nil
		})
	}
	if werr := g.Wait(); werr != nil && ctx.Err() != nil {
		c.JSON(http.StatusRequestTimeout, gin.H{"error": ctx.Err().Error()})
		return
	}

	ok := make([]ResearchPage, 0, n)
	failed := make([]ResearchPage, 0, n)
	paid := false
	for _, p := range pages {
		if p.Error != "" {
			failed = append(failed, p)
			continue
		}
		ok = append(ok, p)
	}
	for _, r := range results {
		if r.Engine == "brave" || r.Engine == "serper" || r.Engine == "tavily" {
			paid = true
			break
		}
	}

	c.JSON(http.StatusOK, ResearchResponse{
		Query:       req.Query,
		Results:     results,
		Pages:       ok,
		Failures:    failed,
		ElapsedMS:   time.Since(started).Milliseconds(),
		PaidAPIUsed: paid,
	})
}

// searchErrorResponse maps a search failure to a truthful status. A challenge
// from an engine is an upstream refusal that needs a human in the open tab, which
// is a 502 with a branchable code — not a 500, which would say our server broke.
func searchErrorResponse(c *gin.Context, err error) {
	if errors.Is(err, search.ErrBrowserChallenge) {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
			"code":  "awaiting-human",
			"hint":  "clear the check in the bridge tab, then retry the query",
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func (h *SearchHandler) Search(c *gin.Context) {
	var req SearchRequest

	// Try to bind from JSON first (POST)
	if c.Request.Method == http.MethodPost && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Parse parameters from query params (works for both GET and POST)
	if q := c.Query("query"); q != "" {
		req.Query = q
	} else if q := c.Query("q"); q != "" { // Support 'q' as common alias
		req.Query = q
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			req.Offset = offset
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}
	if cat := c.Query("category"); cat != "" {
		req.Category = cat
	}
	if r := c.Query("rerank"); r != "" {
		if b, err := strconv.ParseBool(r); err == nil {
			req.Rerank = b
		} else if r == "1" {
			req.Rerank = true
		}
	}
	if r := c.Query("scrapeContent"); r != "" {
		if b, err := strconv.ParseBool(r); err == nil {
			req.ScrapeContent = b
		} else if r == "1" {
			req.ScrapeContent = true
		}
	}

	// Validate query
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter is required"})
		return
	}
	if err := search.ValidateCategory(req.Category); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 10
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	engine, engineErr := resolveEngine(req.Engine, c.Query("engine"))
	if engineErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": engineErr.Error()})
		return
	}

	results, totalCount, err := h.search(c.Request.Context(), search.SearchOptions{
		Query:          req.Query,
		Offset:         req.Offset,
		Limit:          req.Limit,
		IncludeDomains: req.IncludeDomains,
		ExcludeDomains: req.ExcludeDomains,
		RequiredText:   req.RequiredText,
		MaxAge:         req.MaxAge,
		Mode:           req.Mode,
		Category:       req.Category,
		Rerank:         req.Rerank,
		BrowserEngine:  engine,
	})
	if err != nil {
		searchErrorResponse(c, err)
		return
	}

	hasMore := req.Offset+req.Limit < totalCount
	nextOffset := req.Offset + req.Limit

	// Content composition happens after the cached search layer, so the SERP
	// cache stays keyed on the query alone and the scrape cache is reused
	// independently on repeat calls.
	if req.ScrapeContent && h.scraper != nil {
		results = search.EnrichResults(c.Request.Context(), results, h.scraper, search.EnrichOptions{
			Limit: req.ScrapeLimit,
			Mode:  req.Mode,
		})
	}

	c.JSON(http.StatusOK, SearchResponse{
		Query:      req.Query,
		Results:    results,
		HasMore:    hasMore,
		NextOffset: nextOffset,
		Count:      len(results),
	})
}
