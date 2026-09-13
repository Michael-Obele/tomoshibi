package handlers

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/worker"
)

type CrawlRequest struct {
	URL           string   `json:"url" binding:"required,url"`
	Render        bool     `json:"render"`
	Screenshot    bool     `json:"screenshot"`
	Images        bool     `json:"images"`
	ImageFormat   string   `json:"image_format"`
	MaxDepth      int      `json:"maxDepth"`
	Limit         int      `json:"limit"`
	IncludePaths  []string `json:"include_paths"`
	ExcludePaths  []string `json:"exclude_paths"`
	WebhookURL    string   `json:"webhook_url"`
	WebhookSecret string   `json:"webhook_secret"`
}

type CrawlResponse struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Render      bool   `json:"render"`
	Screenshot  bool   `json:"screenshot"`
	Images      bool   `json:"images"`
	ImageFormat string `json:"image_format,omitempty"`
	MaxDepth    int    `json:"maxDepth"`
	Limit       int    `json:"limit"`
}

type CrawlHandler struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

func NewCrawlHandler(redisAddr string) (*CrawlHandler, error) {
	u, err := url.Parse(redisAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	password, _ := u.User.Password()
	addr := u.Host

	redisOpt := asynq.RedisClientOpt{
		Addr:     addr,
		Password: password,
	}

	if u.Scheme == "rediss" {
		redisOpt.TLSConfig = &tls.Config{
			InsecureSkipVerify: false, // Set to true for self-signed certs
			MinVersion:         tls.VersionTLS12,
		}
	}

	client := asynq.NewClient(redisOpt)
	inspector := asynq.NewInspector(redisOpt)
	return &CrawlHandler{
		client:    client,
		inspector: inspector,
	}, nil
}

func (h *CrawlHandler) Close() {
	h.client.Close()
	h.inspector.Close()
}

// EnqueueCrawl godoc
// @Summary      Enqueue a URL for asynchronous crawling
// @Description  Submits a URL to be crawled asynchronously. The crawler performs BFS link-following up to maxDepth, scraping up to limit pages on the same domain. Requires Redis.
// @Tags         crawl
// @Accept       json
// @Produce      json
// @Param        body   body      CrawlRequest   true  "JSON request body"
// @Success      202    {object}  CrawlResponse
// @Failure      400    {object}  map[string]interface{}
// @Failure      500    {object}  map[string]interface{}
// @Router       /crawl [post]
func (h *CrawlHandler) EnqueueCrawl(c *gin.Context) {
	var req CrawlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Apply defaults
	if req.MaxDepth <= 0 {
		req.MaxDepth = 2
	}
	if req.MaxDepth > 10 {
		req.MaxDepth = 10
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	task, err := worker.NewCrawlTaskWithOptions(req.URL, req.Render, req.Screenshot, req.Images, req.ImageFormat, req.MaxDepth, req.Limit, worker.CrawlOptions{
		IncludePaths:  req.IncludePaths,
		ExcludePaths:  req.ExcludePaths,
		WebhookURL:    req.WebhookURL,
		WebhookSecret: req.WebhookSecret,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	info, err := h.client.Enqueue(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue task"})
		return
	}

	c.JSON(http.StatusAccepted, CrawlResponse{
		ID:          info.ID,
		URL:         req.URL,
		Render:      req.Render,
		Screenshot:  req.Screenshot,
		Images:      req.Images,
		ImageFormat: req.ImageFormat,
		MaxDepth:    req.MaxDepth,
		Limit:       req.Limit,
	})
}

// GetCrawlStatus godoc
// @Summary      Get the status of an asynchronous crawl
// @Description  Retrieves the current status and result of a previously enqueued crawl task by its ID.
// @Tags         crawl
// @Produce      json
// @Param        id     path      string  true  "The crawl task ID"
// @Success      200    {object}  map[string]interface{} "The task status and result payload"
// @Failure      404    {object}  map[string]interface{} "Task not found"
// @Router       /crawl/{id} [get]
func (h *CrawlHandler) GetCrawlStatus(c *gin.Context) {
	id := c.Param("id")

	// Search across all configured queues
	queues := []string{"default", "critical", "low"}
	var info *asynq.TaskInfo

	for _, q := range queues {
		ti, err := h.inspector.GetTaskInfo(q, id)
		if err == nil {
			info = ti
			break
		}
		// A task genuinely absent from a queue is not an error worth
		// surfacing — try the next one. That includes a queue that has no
		// Redis keys yet (ErrQueueNotFound): a task cannot be in a queue
		// that does not exist. Anything else (Redis down, pool exhausted,
		// timeout) is transient: the task may well exist, and reporting 404
		// here is a false negative that makes clients think a job vanished.
		// Fail with 503 instead so the client retries.
		if !errors.Is(err, asynq.ErrTaskNotFound) && !errors.Is(err, asynq.ErrQueueNotFound) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":  "task status temporarily unavailable",
				"detail": err.Error(),
			})
			return
		}
	}

	if info == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Build response with task metadata
	resp := gin.H{
		"id":    info.ID,
		"queue": info.Queue,
		"state": taskStateLabel(info.State),
	}

	// Parse and format the result when the task has completed
	if info.State == asynq.TaskStateCompleted && len(info.Result) > 0 {
		var crawlResult worker.CrawlResult
		if err := json.Unmarshal(info.Result, &crawlResult); err == nil {
			// Build a clean pages list with titles and previews
			type pageEntry struct {
				URL     string `json:"url"`
				Title   string `json:"title,omitempty"`
				Preview string `json:"preview,omitempty"`
			}
			pages := make([]pageEntry, 0, len(crawlResult.Pages))
			for _, p := range crawlResult.Pages {
				pages = append(pages, pageEntry{
					URL:     p.URL,
					Title:   extractTitle(p),
					Preview: extractPreview(p.Markdown, 300),
				})
			}

			resp["crawl"] = gin.H{
				"status":      crawlResult.Status,
				"total_pages": crawlResult.TotalPages,
				"max_depth":   crawlResult.MaxDepth,
				"limit":       crawlResult.Limit,
				"pages":       pages,
			}
			if len(crawlResult.FailedURLs) > 0 {
				resp["failed_urls"] = crawlResult.FailedURLs
			}
		} else {
			// Fallback: return raw result if parsing fails
			resp["result"] = string(info.Result)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// taskStateLabel returns a human-readable state label for an Asynq task.
func taskStateLabel(s asynq.TaskState) string {
	switch s {
	case asynq.TaskStateActive:
		return "active"
	case asynq.TaskStatePending:
		return "pending"
	case asynq.TaskStateScheduled:
		return "scheduled"
	case asynq.TaskStateRetry:
		return "retry"
	case asynq.TaskStateArchived:
		// Retries exhausted — a permanent failure. Map to "failed" so
		// clients (and MCP tools) that don't know the internal "archived"
		// state can treat the job as finished.
		return "failed"
	case asynq.TaskStateCompleted:
		return "completed"
	default:
		return s.String()
	}
}

// extractTitle returns the best available page title. It falls back through
// the HTML `<h1>`, the `<title>` tag, `meta[property="og:title"]`, the first
// heading of any level, the first markdown H1, and finally the URL hostname,
// so a title is almost always returned.
func extractTitle(p domain.ScrapeResult) string {
	if p.HTML != "" {
		if doc, err := goquery.NewDocumentFromReader(strings.NewReader(p.HTML)); err == nil {
			if t := strings.TrimSpace(doc.Find("h1").First().Text()); t != "" {
				return t
			}
			if t := strings.TrimSpace(doc.Find("title").First().Text()); t != "" {
				return t
			}
			if t := strings.TrimSpace(doc.Find(`meta[property="og:title"]`).First().AttrOr("content", "")); t != "" {
				return t
			}
			if t := strings.TrimSpace(doc.Find("h1,h2,h3,h4,h5,h6").First().Text()); t != "" {
				return t
			}
		}
	}
	// Markdown fallback: the first H1 line.
	for _, line := range strings.Split(p.Markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}
	// Last resort: the hostname of the scraped URL.
	if u, err := url.Parse(p.URL); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return ""
}

// extractPreview returns the first N characters of clean markdown content.
func extractPreview(markdown string, maxLen int) string {
	cleaned := strings.TrimSpace(markdown)
	runes := []rune(cleaned)
	if len(runes) <= maxLen {
		return cleaned
	}
	// Cut at the last space before maxLen to avoid word-splitting, looking back
	// at most 50 runes. The floor is clamped at zero: a maxLen below the
	// lookback window would otherwise let cut walk past the start of the string
	// and index runes[-1].
	floor := maxLen - 50
	if floor < 0 {
		floor = 0
	}
	cut := maxLen
	for cut > floor && cut < len(runes) && runes[cut] != ' ' {
		cut--
	}
	if cut <= floor {
		// No space within the window — cut at the limit rather than walking
		// back arbitrarily far.
		cut = maxLen
	}
	return string(runes[:cut]) + "..."
}
