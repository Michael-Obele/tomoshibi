package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Michael-Obele/tomoshibi/internal/sitemap"
)

// MapRequest is the request body for POST /v1/map.
type MapRequest struct {
	URL    string `json:"url" binding:"required,url"`
	Search string `json:"search,omitempty"` // optional substring filter
	Limit  int    `json:"limit,omitempty"`  // default 100, max 5000
}

// MapResponse is the payload returned by POST /v1/map.
type MapResponse struct {
	URL   string                  `json:"url"`
	Count int                     `json:"count"`
	Links []sitemap.DiscoveredURL `json:"links"`
}

// mapTimeout bounds sitemap discovery per request.
const mapTimeout = 30 * time.Second

// MapHandler serves the /v1/map endpoint.
type MapHandler struct{}

// NewMapHandler creates a MapHandler.
func NewMapHandler() *MapHandler {
	return &MapHandler{}
}

// Map godoc
// @Summary      Map a website's URLs
// @Description  Discovers URLs for a site via robots.txt/sitemap.xml, falling back to link discovery. Returns up to limit URLs, optionally filtered by search.
// @Tags         map
// @Accept       json
// @Produce      json
// @Param        body   body      MapRequest  true  "JSON request body"
// @Success      200    {object}  MapResponse
// @Failure      400    {object}  map[string]interface{}
// @Failure      500    {object}  map[string]interface{}
// @Router       /map [post]
func (h *MapHandler) Map(c *gin.Context) {
	var req MapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 5000 {
		limit = 5000
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), mapTimeout)
	defer cancel()

	found, err := sitemap.Discover(ctx, req.URL, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	links := make([]sitemap.DiscoveredURL, 0, len(found))
	for _, u := range found {
		if req.Search != "" && !strings.Contains(u.URL, req.Search) {
			continue
		}
		links = append(links, u)
	}

	c.JSON(http.StatusOK, MapResponse{
		URL:   req.URL,
		Count: len(links),
		Links: links,
	})
}
