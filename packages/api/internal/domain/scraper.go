package domain

import "context"

// LinkData represents one extracted hyperlink, mirroring Firecrawl's `links` format.
// Firecrawl (self-hosted at http://localhost:3002) returns `links: ["https://..."]` as strings;
// Cinder enriches each entry with anchor text and a same-host flag for parity.
type LinkData struct {
	URL        string `json:"url"`
	Text       string `json:"text,omitempty"`
	IsInternal bool   `json:"isInternal"`
}

type ScrapeResult struct {
	URL      string            `json:"url"`
	Markdown string            `json:"markdown"`
	HTML     string            `json:"html,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`

	// Image fields (omitted when not requested)
	Screenshot *ScreenshotData `json:"screenshot,omitempty"`
	Images     []ImageData     `json:"images,omitempty"`

	// Links holds extracted hyperlinks (after readability, deduped, absolute).
	Links []LinkData `json:"links,omitempty"`

	// Extracted holds deterministic schema-extraction results.
	Extracted map[string]any `json:"extracted,omitempty"`

	// Summary holds the extractive summary when requested.
	Summary string `json:"summary,omitempty"`
}

// ExtractField describes one deterministic extraction rule.
type ExtractField struct {
	Selector string `json:"selector"`
	Attr     string `json:"attr,omitempty"` // "text" (default), "html", or attribute name
	Multiple bool   `json:"multiple,omitempty"`
}

type ScrapeOptions struct {
	Mode               string                  `json:"mode,omitempty"`
	Screenshot         bool                    `json:"screenshot"`
	Images             bool                    `json:"images"`
	ImageFormat        ImageTransportFormat    `json:"image_format,omitempty"`
	ScreenshotOpts     *ScreenshotOptions      `json:"screenshot_opts,omitempty"`
	MaxImages          int                     `json:"max_images,omitempty"`
	MaxImageSizeKB     int                     `json:"max_image_size_kb,omitempty"`
	ImageProcess       *ImageProcessOptions    `json:"image_process,omitempty"`
	Actions            []Action                `json:"actions,omitempty"`
	ExtractSchema      map[string]ExtractField `json:"extract_schema,omitempty"`
	Summary            bool                    `json:"summary,omitempty"`
	SummarySentences   int                     `json:"summary_sentences,omitempty"`
	RedactPII          bool                    `json:"redact_pii,omitempty"`
	BlockAds           *bool                   `json:"block_ads,omitempty"`
	RemoveBase64Images *bool                   `json:"remove_base64_images,omitempty"`
	IncludeLinks       *bool                   `json:"include_links,omitempty"`
}

// Action is a single page interaction executed before content capture
// (dynamic mode only). Supported types: wait_ms, wait_selector, click,
// scroll_down, scroll_to_bottom.
type Action struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
	Ms       int    `json:"ms,omitempty"`
}

type Scraper interface {
	Scrape(ctx context.Context, url string, opts ScrapeOptions) (*ScrapeResult, error)
}
