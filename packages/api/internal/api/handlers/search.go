package handlers

import (
	"net/http"
	"strconv"

	"github.com/Michael-Obele/tomoshibi/internal/search"
	"github.com/gin-gonic/gin"
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
}

func NewSearchHandler(s search.Service) *SearchHandler {
	return &SearchHandler{
		service: s,
	}
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
// @Param        body           body      SearchRequest  false  "JSON request body"
// @Success      200    {object}  SearchResponse
// @Failure      400    {object}  map[string]interface{}
// @Failure      500    {object}  map[string]interface{}
// @Router       /search [post]
// @Router       /search [get]
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

	results, totalCount, err := h.service.Search(c.Request.Context(), search.SearchOptions{
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
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hasMore := req.Offset+req.Limit < totalCount
	nextOffset := req.Offset + req.Limit

	c.JSON(http.StatusOK, SearchResponse{
		Query:      req.Query,
		Results:    results,
		HasMore:    hasMore,
		NextOffset: nextOffset,
		Count:      len(results),
	})
}
