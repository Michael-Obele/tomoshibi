package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TypeScrape = "scrape:url"
	TypeCrawl  = "crawl:site"
)

type ScrapePayload struct {
	URL         string `json:"url"`
	Render      bool   `json:"render"` // Deprecated: usage ignores Mode if true
	Mode        string `json:"mode"`   // "smart", "static", "dynamic"
	Screenshot  bool   `json:"screenshot"`
	Images      bool   `json:"images"`
	ImageFormat string `json:"image_format,omitempty"`
}

// CrawlPayload extends ScrapePayload with multi-page crawling options.
type CrawlPayload struct {
	URL           string   `json:"url"`
	Render        bool     `json:"render"`
	Mode          string   `json:"mode"`
	Screenshot    bool     `json:"screenshot"`
	Images        bool     `json:"images"`
	ImageFormat   string   `json:"image_format,omitempty"`
	MaxDepth      int      `json:"maxDepth"`
	Limit         int      `json:"limit"`
	IncludePaths  []string `json:"include_paths,omitempty"`
	ExcludePaths  []string `json:"exclude_paths,omitempty"`
	WebhookURL    string   `json:"webhook_url,omitempty"`
	WebhookSecret string   `json:"webhook_secret,omitempty"`
}

// NewScrapeTask creates a new task for scraping a single URL.
func NewScrapeTask(
	url string,
	render bool,
	screenshot bool,
	images bool,
	imageFormat string,
) (*asynq.Task, error) {
	payload := ScrapePayload{
		URL:         url,
		Render:      render,
		Screenshot:  screenshot,
		Images:      images,
		ImageFormat: imageFormat,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scrape payload: %w", err)
	}
	// Keep result for 7 days
	return asynq.NewTask(TypeScrape, data, asynq.Retention(7*24*time.Hour)), nil
}

// CrawlOptions carries optional crawl configuration applied at enqueue time.
type CrawlOptions struct {
	IncludePaths  []string
	ExcludePaths  []string
	WebhookURL    string
	WebhookSecret string
}

// NewCrawlTask creates a new task for crawling a site starting from a seed URL.
func NewCrawlTask(
	crawlURL string,
	render bool,
	screenshot bool,
	images bool,
	imageFormat string,
	maxDepth int,
	limit int,
) (*asynq.Task, error) {
	return NewCrawlTaskWithOptions(crawlURL, render, screenshot, images, imageFormat, maxDepth, limit, CrawlOptions{})
}

// NewCrawlTaskWithOptions is NewCrawlTask plus include/exclude patterns and
// an optional completion webhook.
func NewCrawlTaskWithOptions(
	crawlURL string,
	render bool,
	screenshot bool,
	images bool,
	imageFormat string,
	maxDepth int,
	limit int,
	opts CrawlOptions,
) (*asynq.Task, error) {
	payload := CrawlPayload{
		URL:           crawlURL,
		Render:        render,
		Screenshot:    screenshot,
		Images:        images,
		ImageFormat:   imageFormat,
		MaxDepth:      maxDepth,
		Limit:         limit,
		IncludePaths:  opts.IncludePaths,
		ExcludePaths:  opts.ExcludePaths,
		WebhookURL:    opts.WebhookURL,
		WebhookSecret: opts.WebhookSecret,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal crawl payload: %w", err)
	}
	// Crawl tasks can be longer; retain result for 7 days.
	// The task-level timeout is a safety net BEYOND the internal deadline in
	// ExecuteCrawl (CRAWL_TIMEOUT): if the handler ever hangs (a bug), Asynq
	// kills it after this and retries/archives per MaxRetry instead of
	// occupying the worker forever.
	return asynq.NewTask(TypeCrawl, data,
		asynq.Retention(7*24*time.Hour),
		asynq.MaxRetry(2),
		asynq.Timeout(crawlTimeout()+5*time.Minute),
	), nil
}
