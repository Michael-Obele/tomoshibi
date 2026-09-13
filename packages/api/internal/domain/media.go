package domain

import "time"

// BlobData represents a self-contained image with inline data.
// Designed for direct consumption by AI APIs (OpenAI, Gemini, Anthropic).
type BlobData struct {
	// DataURI is the complete data URI: "data:image/png;base64,iVBORw0KGgo..."
	DataURI  string `json:"blob,omitempty"`
	MimeType string `json:"mime_type"`
	RawBytes []byte `json:"-"`
}

// ImageData represents a single image found on a scraped page.
type ImageData struct {
	URL        string `json:"url"`
	Blob       string `json:"blob,omitempty"`
	Alt        string `json:"alt,omitempty"`
	Title      string `json:"title,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Format     string `json:"format,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	SourceType string `json:"source,omitempty"`
}

// ScreenshotData represents a captured page screenshot.
type ScreenshotData struct {
	Blob       string    `json:"blob,omitempty"`
	URL        string    `json:"url,omitempty"`
	Format     string    `json:"format"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	FullPage   bool      `json:"full_page"`
	SizeBytes  int64     `json:"size_bytes"`
	CapturedAt time.Time `json:"captured_at"`
}

// ScreenshotOptions configures screenshot capture behavior.
type ScreenshotOptions struct {
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	FullPage     bool   `json:"full_page,omitempty"`
	Format       string `json:"format,omitempty"` // "jpeg" (default) or "png"
	Quality      int    `json:"quality,omitempty"`
	WaitSelector string `json:"wait_selector,omitempty"`
}

// ImageTransportFormat controls how images are returned.
type ImageTransportFormat string

const (
	ImageFormatURL  ImageTransportFormat = "url"
	ImageFormatBlob ImageTransportFormat = "blob"
)

// ImageProcessOptions configures optional image resizing/re-encoding.
type ImageProcessOptions struct {
	Format   string `json:"format,omitempty"`    // "jpeg" (default) or "png"
	MaxWidth int    `json:"max_width,omitempty"` // 0 = no resize
	Quality  int    `json:"quality,omitempty"`   // 1-100, 0 = default 80
}
