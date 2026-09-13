package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
	"golang.org/x/sync/errgroup"
)

type ScrapeRequest struct {
	URL                string                         `json:"url,omitempty"`
	URLs               []string                       `json:"urls,omitempty"`
	Render             bool                           `json:"render"` // Deprecated: usage ignores Mode if true
	Mode               string                         `json:"mode"`   // "smart", "static", "dynamic"
	Screenshot         bool                           `json:"screenshot"`
	ScreenshotOpts     *ScreenshotOpts                `json:"screenshot_opts,omitempty"`
	Images             bool                           `json:"images"`
	ImageFormat        string                         `json:"image_format"` // "url" or "blob"
	MaxImages          int                            `json:"max_images"`
	MaxImageSizeKB     int                            `json:"max_image_size_kb"`
	ImageProcess       *ImageProcessReq               `json:"image_process,omitempty"`
	Actions            []ActionReq                    `json:"actions,omitempty"`
	ExtractSchema      map[string]domain.ExtractField `json:"extract_schema,omitempty"`
	Summary            bool                           `json:"summary,omitempty"`
	SummarySentences   int                            `json:"summary_sentences,omitempty"`
	RedactPII          bool                           `json:"redact_pii,omitempty"`
	BlockAds           *bool                          `json:"block_ads,omitempty"`
	RemoveBase64Images *bool                          `json:"remove_base64_images,omitempty"`
	IncludeLinks       *bool                          `json:"include_links,omitempty"`
}

// maxMultiScrapeURLs caps sync multi-URL scrape, mirroring web_fetch_exa.
const maxMultiScrapeURLs = 10

// MultiScrapeItem is one entry in a multi-URL scrape response.
// It mirrors the single-URL response shape so callers can treat each item uniformly.
type MultiScrapeItem struct {
	URL        string                 `json:"url"`
	Title      string                 `json:"title,omitempty"`
	WordCount  int                    `json:"word_count,omitempty"`
	Markdown   string                 `json:"markdown,omitempty"`
	HTML       string                 `json:"html,omitempty"`
	Metadata   map[string]string      `json:"metadata,omitempty"`
	Screenshot *domain.ScreenshotData `json:"screenshot,omitempty"`
	Images     []domain.ImageData     `json:"images,omitempty"`
	Links      []domain.LinkData      `json:"links,omitempty"`
	Extracted  map[string]any         `json:"extracted,omitempty"`
	Summary    string                 `json:"summary,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// MultiScrapeResponse is returned when `urls` is used.
type MultiScrapeResponse struct {
	Results []MultiScrapeItem `json:"results"`
}

// ScreenshotOpts is the wire format for screenshot configuration.
type ScreenshotOpts struct {
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	FullPage     bool   `json:"full_page,omitempty"`
	Format       string `json:"format,omitempty"`
	Quality      int    `json:"quality,omitempty"`
	WaitSelector string `json:"wait_selector,omitempty"`
}

// ImageProcessReq is the wire format for image resizing/re-encoding.
type ImageProcessReq struct {
	Format   string `json:"format,omitempty"` // "jpeg" (default) or "png"
	MaxWidth int    `json:"max_width,omitempty"`
	Quality  int    `json:"quality,omitempty"`
}

// ActionReq is the wire format for a page action.
type ActionReq struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
	Ms       int    `json:"ms,omitempty"`
}

type ScrapeHandler struct {
	service *scraper.Service
}

func NewScrapeHandler(s *scraper.Service) *ScrapeHandler {
	return &ScrapeHandler{service: s}
}

// mapScreenshotOpts converts the wire-format screenshot options into the
// domain type, returning nil when nothing was provided.
func mapScreenshotOpts(in *ScreenshotOpts) *domain.ScreenshotOptions {
	if in == nil {
		return nil
	}
	return &domain.ScreenshotOptions{
		Width:        in.Width,
		Height:       in.Height,
		FullPage:     in.FullPage,
		Format:       in.Format,
		Quality:      in.Quality,
		WaitSelector: in.WaitSelector,
	}
}

// mapImageProcess converts the wire-format image process options into the
// domain type, returning nil when nothing was provided.
func mapImageProcess(in *ImageProcessReq) *domain.ImageProcessOptions {
	if in == nil {
		return nil
	}
	return &domain.ImageProcessOptions{
		Format:   in.Format,
		MaxWidth: in.MaxWidth,
		Quality:  in.Quality,
	}
}

// mapActions converts wire-format actions into domain actions.
func mapActions(in []ActionReq) []domain.Action {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Action, 0, len(in))
	for _, a := range in {
		out = append(out, domain.Action{Type: a.Type, Selector: a.Selector, Ms: a.Ms})
	}
	return out
}

// wordCount returns the number of whitespace-delimited words in the given text.
func wordCount(text string) int {
	if text == "" {
		return 0
	}
	var count int
	var inWord bool
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			count++
			inWord = true
		}
	}
	return count
}

// isValidURL reports whether s is a parseable http(s) URL.
func isValidURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// Scrape godoc
// @Summary      Scrape a webpage
// @Description  Scrapes a given URL and returns its markdown content, metadata, links, and optionally captures a screenshot or extracts images if enabled. Links are extracted from readability DOM, resolved to absolute URLs, deduped, and flagged isInternal via same-host check — matching Firecrawl `formats: ["links"]` (Firecrawl returns `links: ["https://..."]`; Cinder enriches to `links: [{url, text, isInternal}]`). Include_links defaults to true. Accepts either `url` (single) or `urls` (up to 10) for synchronous multi-URL scrape; when `urls` is used the response is `{results: [{url, markdown, metadata, ...}]}`. Mirrors Firecrawl `POST /v2/scrape` (single) and `POST /v2/batch/scrape` (multi) and the `web_fetch_exa` multi-URL shape — see https://docs.firecrawl.dev/features/scrape and https://docs.firecrawl.dev/api-reference/endpoint/scrape. Research via http://localhost:3002/v1/scrape against those docs showed `links` as string array; Cinder returns objects with text/isInternal.
// @Tags         scrape
// @Accept       json
// @Produce      json
// @Param        url              query     string  false  "The URL to scrape (single mode)"
// @Param        urls             query     string  false  "JSON array alternative: use POST body {\"urls\": [\"https://a.com\",\"https://b.com\"]} for batch sync (max 10)"
// @Param        mode             query     string  false  "Scraping mode: smart, static, dynamic"
// @Param        render           query     bool    false  "Deprecated: use mode=dynamic instead"
// @Param        screenshot       query     bool    false  "Capture full-page screenshot (requires mode=dynamic or smart)"
// @Param        images           query     bool    false  "Extract images from the page"
// @Param        image_format     query     string  false  "Image transport: 'url' (default, metadata only) or 'blob' (base64 data URIs)"
// @Param        max_images       query     int     false  "Maximum images to extract (default: 10)"
// @Param        max_image_size_kb query    int     false  "Max individual image size in KB (default: 5120)"
// @Param        include_links    query     bool    false  "Include extracted links (default true)"
// @Param        body             body      ScrapeRequest  false  "JSON request body (alternative to query params). Provide either `url` or `urls` (max 10, exclusive)."
// @Success      200    {object}  domain.ScrapeResult  "single URL"
// @Success      200    {object}  MultiScrapeResponse  "multi URL (when urls is provided)"
// @Failure      400    {object}  map[string]interface{}
// @Failure      500    {object}  map[string]interface{}
// @Router       /scrape [post]
// @Router       /scrape [get]
func (h *ScrapeHandler) Scrape(c *gin.Context) {
	var req ScrapeRequest

	// Try to bind from JSON first (POST)
	if c.Request.Method == http.MethodPost && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Log.Warn("Invalid check", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
			return
		}
	}

	// Parse parameters from query strings (GET or POST)
	if url := c.Query("url"); url != "" {
		req.URL = url
	}
	if mode := c.Query("mode"); mode != "" {
		req.Mode = mode
	}
	if render := c.Query("render"); render == "true" {
		req.Render = true
	}
	if screenshot := c.Query("screenshot"); screenshot == "true" {
		req.Screenshot = true
	}
	if images := c.Query("images"); images == "true" {
		req.Images = true
	}
	if imageFormat := c.Query("image_format"); imageFormat != "" {
		req.ImageFormat = imageFormat
	}
	if maxImagesStr := c.Query("max_images"); maxImagesStr != "" {
		if maxImages, err := strconv.Atoi(maxImagesStr); err == nil {
			req.MaxImages = maxImages
		}
	}
	if maxSizeStr := c.Query("max_image_size_kb"); maxSizeStr != "" {
		if maxSize, err := strconv.Atoi(maxSizeStr); err == nil {
			req.MaxImageSizeKB = maxSize
		}
	}
	if includeLinksStr := c.Query("include_links"); includeLinksStr != "" {
		v := includeLinksStr == "true" || includeLinksStr == "1"
		if includeLinksStr == "false" || includeLinksStr == "0" {
			v = false
		} else if includeLinksStr != "true" && includeLinksStr != "1" {
			// Treat any non-false value as true for truthy query params
			v = true
		}
		req.IncludeLinks = &v
	}

	// Validate exclusive url / urls and max constraints
	hasURL := req.URL != ""
	hasURLs := len(req.URLs) > 0
	if hasURL && hasURLs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provide either 'url' or 'urls', not both"})
		return
	}
	if !hasURL && !hasURLs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}
	if hasURLs {
		if len(req.URLs) > maxMultiScrapeURLs {
			c.JSON(http.StatusBadRequest, gin.H{"error": "too many URLs (max 10)"})
			return
		}
		for _, u := range req.URLs {
			if !isValidURL(u) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid URL in urls: " + u})
				return
			}
		}
	} else {
		if !isValidURL(req.URL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid URL"})
			return
		}
	}

	// Backward compatibility mapping
	mode := req.Mode
	if req.Render {
		mode = "dynamic"
	}
	if mode == "" {
		mode = "smart"
	}

	// Parse image format
	imageFormat := domain.ImageFormatURL
	switch req.ImageFormat {
	case "blob":
		imageFormat = domain.ImageFormatBlob
	case "url":
		imageFormat = domain.ImageFormatURL
	}

	// include_links defaults to true for Firecrawl parity.
	if req.IncludeLinks == nil {
		t := true
		req.IncludeLinks = &t
	}
	opts := domain.ScrapeOptions{
		Screenshot:         req.Screenshot,
		ScreenshotOpts:     mapScreenshotOpts(req.ScreenshotOpts),
		Images:             req.Images,
		ImageFormat:        imageFormat,
		MaxImages:          req.MaxImages,
		MaxImageSizeKB:     req.MaxImageSizeKB,
		ImageProcess:       mapImageProcess(req.ImageProcess),
		Actions:            mapActions(req.Actions),
		ExtractSchema:      req.ExtractSchema,
		Summary:            req.Summary,
		SummarySentences:   req.SummarySentences,
		RedactPII:          req.RedactPII,
		BlockAds:           req.BlockAds,
		RemoveBase64Images: req.RemoveBase64Images,
		IncludeLinks:       req.IncludeLinks,
	}

	// Multi-URL sync path (sync, errgroup limit 5, reuse Service.Scrape)
	if hasURLs {
		logger.Log.Info("Multi-scrape request", "count", len(req.URLs), "mode", mode)
		results := make([]MultiScrapeItem, len(req.URLs))
		g, ctx := errgroup.WithContext(c.Request.Context())
		g.SetLimit(5)
		for i, u := range req.URLs {
			i, u := i, u
			g.Go(func() error {
				res, err := h.service.Scrape(ctx, u, mode, opts)
				if err != nil {
					if ctx.Err() != nil {
						return err
					}
					logger.Log.Warn("Multi-scrape item failed", "url", u, "error", err)
					results[i] = MultiScrapeItem{URL: u, Error: err.Error()}
					return nil
				}
				var title string
				if res.Markdown != "" || res.HTML != "" {
					title = extractTitle(*res)
				}
				results[i] = MultiScrapeItem{
					URL:        res.URL,
					Title:      title,
					WordCount:  wordCount(res.Markdown),
					Markdown:   res.Markdown,
					HTML:       res.HTML,
					Metadata:   res.Metadata,
					Screenshot: res.Screenshot,
					Images:     res.Images,
					Links:      res.Links,
					Extracted:  res.Extracted,
					Summary:    res.Summary,
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			logger.Log.Error("Multi-scrape aborted", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
		return
	}

	result, err := h.service.Scrape(c.Request.Context(), req.URL, mode, opts)
	if err != nil {
		logger.Log.Error("Scrape failed", "url", req.URL, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Extract title and word count for the summary
	var title string
	if result.Markdown != "" || result.HTML != "" {
		title = extractTitle(*result)
	}

	c.JSON(http.StatusOK, gin.H{
		"url":        result.URL,
		"title":      title,
		"word_count": wordCount(result.Markdown),
		"markdown":   result.Markdown,
		"html":       result.HTML,
		"metadata":   result.Metadata,
		"screenshot": result.Screenshot,
		"images":     result.Images,
		"links":      result.Links,
		"extracted":  result.Extracted,
		"summary":    result.Summary,
	})
}
