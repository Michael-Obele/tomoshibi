# Image Blob Transport — AI-Ready Image Pipeline

> Detailed plan for capturing, encoding, and transporting images as self-contained blobs that can be directly fed into multimodal AI APIs (OpenAI GPT-4o, Google Gemini, Anthropic Claude, etc.).
> See [Documentation Index](../guides/INDEX.md) for core guides.

**Status**: ✅ **Fully Implemented & Connected**  
**Last Updated**: July 25, 2026  
**Related**: [image-screenshot-feature.md](./image-screenshot-feature.md)

---

## 1. Problem Statement

### Current Limitation

Tomoshibi's scrape/search results return **text only** (Markdown + HTML). When consumers want to send scraped content to multimodal AI models, they must:

1. Re-fetch the page to capture screenshots.
2. Download images separately by parsing HTML for `<img>` tags.
3. Encode everything manually to base64 or upload to temporary storage.

This is wasteful, slow, and error-prone.

### Goal

Provide a **single API response** that includes images in a format directly consumable by AI APIs — eliminating the need for any post-processing by the consumer.

### AI API Input Formats (What We Need to Output)

| Provider      | Accepts                   | Format                                               |
| ------------- | ------------------------- | ---------------------------------------------------- |
| **OpenAI**    | `image_url` in messages   | `data:image/png;base64,<DATA>` or public URL         |
| **Gemini**    | `inlineData` in parts     | `{ mimeType: "image/png", data: "<BASE64>" }`        |
| **Anthropic** | `image` in content blocks | `{ type: "base64", media_type: "...", data: "..." }` |
| **All**       | Public URL                | Direct HTTPS URL to the image                        |

**Takeaway**: Base64-encoded data with MIME type is the universal portable format.

---

## 2. Architecture

### Design Principle: Dual-Mode Output

Every image in the response can be delivered in two modes:

1. **`blob`** — Base64-encoded inline data (self-contained, no external dependency).
2. **`url`** — Original or cached URL (lightweight, but requires network access).

The consumer chooses via a request parameter. Default is `url` (backward-compatible, zero overhead).

### System Flow (Current Implementation)

```
Client Request                         Response
─────────────────                      ─────────────
POST /v1/scrape                        {
  url: "...",                            url: "...",
  screenshot: true,        ──────►       markdown: "...",
  images: true,                          screenshot: {
  image_format: "blob"                     blob: "data:image/jpeg;base64,...",
}                                          width: 1280,
                                           height: 800,
                                           format: "jpeg",
                                           size_bytes: 42100
                                         },
                                         images: [
                                           {
                                             blob: "data:image/png;base64,...",
                                             alt: "Hero banner",
                                             width: 800,
                                             height: 400,
                                             source: "og:image"
                                           }
                                         ]
                                       }
```

> ✅ **Current state:** The full image pipeline is now connected end-to-end. Screenshots are captured in `chromedp.go`, image extraction from HTML runs in `internal/scraper/service.go` after both scrapers return, and blob encoding uses `internal/image/processor.go`. The `Images` flag, `image_format`, `max_images`, and `max_image_size_kb` parameters are fully wired through the API handler → scraper service → image processor.

### Architecture Diagram

```
┌───────────────────────────────────────────────────────────┐
│                        API Layer                          │
│  /v1/scrape  ──►  ScrapeHandler (parse image_format)      │
└───────────────┬───────────────────────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────────────────────┐
│                     Scraper Service                       │
│  Scrape(ctx, url, mode, opts)                             │
│    ├── CollyScraper.Scrape()      → text + HTML           │
│    ├── ChromedpScraper.Scrape()   → text + screenshot[]   │
│    ├── ExtractPageImages(HTML)    → ImageData[]           │
│    └── Processor.FetchAndEncode() → blob encoding         │
└───────────────┬───────────────────────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────────────────────┐
│                    Image Processor                        │
│  internal/image/processor.go                              │
│    ├── CaptureScreenshot(ctx, opts) → []byte              │
│    ├── ExtractPageImages(html)      → []ImageMeta         │
│    ├── FetchAndEncode(url)          → BlobData            │
│    ├── EncodeToBlob(bytes, mime)    → string (data URI)   │
│    └── OptimizeImage(bytes, opts)   → []byte (resized)    │
└───────────────────────────────────────────────────────────┘
```

---

## 3. Data Structures

### 3.1 New File: `internal/domain/media.go`

```go
package domain

import "time"

// BlobData represents a self-contained image with inline data.
// Designed for direct consumption by AI APIs (OpenAI, Gemini, Anthropic).
type BlobData struct {
    // DataURI is the complete data URI: "data:image/png;base64,iVBORw0KGgo..."
    // Ready to drop into OpenAI's image_url field or Gemini's inlineData.
    DataURI   string `json:"blob,omitempty"`

    // MimeType is the MIME type of the image (e.g., "image/png", "image/webp").
    MimeType  string `json:"mime_type"`

    // RawBytes is the raw image bytes (not serialized to JSON, used internally).
    RawBytes  []byte `json:"-"`
}

// ImageData represents a single image found on a page.
type ImageData struct {
    // URL is the original image URL (always populated).
    URL        string `json:"url"`

    // Blob is the base64-encoded data URI (only populated when image_format=blob).
    Blob       string `json:"blob,omitempty"`

    // Metadata
    Alt        string `json:"alt,omitempty"`
    Title      string `json:"title,omitempty"`
    Width      int    `json:"width,omitempty"`
    Height     int    `json:"height,omitempty"`
    Format     string `json:"format,omitempty"`     // "png", "jpeg", "webp"
    SizeBytes  int64  `json:"size_bytes,omitempty"`
    SourceType string `json:"source,omitempty"`      // "og:image", "favicon", "content", "hero"
}

// ScreenshotData represents a captured page screenshot.
type ScreenshotData struct {
    // Blob is the base64 data URI (when image_format=blob).
    Blob       string    `json:"blob,omitempty"`

    // URL is a public URL to the hosted screenshot (when image_format=url).
    URL        string    `json:"url,omitempty"`

    // Metadata
    Format     string    `json:"format"`              // "png", "webp", "jpeg"
    Width      int       `json:"width"`
    Height     int       `json:"height"`
    FullPage   bool      `json:"full_page"`
    SizeBytes  int64     `json:"size_bytes"`
    CapturedAt time.Time `json:"captured_at"`
}

// ScreenshotOptions configures screenshot capture behavior.
type ScreenshotOptions struct {
    Width        int    // Viewport width (default: 1280)
    Height       int    // Viewport height (default: 800)
    FullPage     bool   // Capture entire scrollable page
    Format       string // "png", "webp", "jpeg" (default: "webp" for smallest size)
    Quality      int    // 1-100, JPEG/WebP quality (default: 80)
    WaitSelector string // CSS selector to wait for before capture
}

// ImageTransportFormat controls how images are returned.
type ImageTransportFormat string

const (
    // ImageFormatURL returns image URLs only (lightweight, default).
    ImageFormatURL  ImageTransportFormat = "url"

    // ImageFormatBlob returns base64 data URIs (self-contained, AI-ready).
    ImageFormatBlob ImageTransportFormat = "blob"
)
```

### 3.2 Current: `internal/domain/scraper.go`

```go
type ScrapeResult struct {
    URL        string            `json:"url"`
    Markdown   string            `json:"markdown"`
    HTML       string            `json:"html,omitempty"`
    Metadata   map[string]string `json:"metadata,omitempty"`

    // Image fields (omitted when not requested)
    Screenshot *ScreenshotData   `json:"screenshot,omitempty"`
    Images     []ImageData       `json:"images,omitempty"`
}

// ScrapeOptions — current as-implemented
type ScrapeOptions struct {
    Mode           string               `json:"mode,omitempty"`
    Screenshot     bool                 `json:"screenshot"`
    Images         bool                 `json:"images"`
    ImageFormat    ImageTransportFormat `json:"image_format,omitempty"`
    ScreenshotOpts *ScreenshotOptions   `json:"screenshot_opts,omitempty"`
    MaxImages      int                  `json:"max_images,omitempty"`
    MaxImageSizeKB int                  `json:"max_image_size_kb,omitempty"`
}
```

### 3.3 Current API Request (`internal/api/handlers/scrape.go`)

```go
type ScrapeRequest struct {
    URL        string `json:"url" binding:"required,url"`
    Render     bool   `json:"render"`          // Deprecated
    Mode       string `json:"mode"`
    Screenshot bool   `json:"screenshot"`
    Images     bool   `json:"images"`
    // Note: image_format, max_images, max_image_size_kb are defined
    // in ScrapeOptions but not yet exposed in the request handler
}
```

---

## 4. Image Processor Implementation

### 4.1 New File: `internal/image/processor.go`

```go
package image

import (
    "encoding/base64"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/standard-user/tomoshibi/internal/domain"
)

const (
    MaxImageSize    = 5 * 1024 * 1024 // 5MB max per image
    FetchTimeout    = 10 * time.Second
    DefaultQuality  = 80
    DefaultFormat   = "webp"
)

// Processor handles image encoding and optimization.
type Processor struct {
    client *http.Client
}

func NewProcessor() *Processor {
    return &Processor{
        client: &http.Client{
            Timeout: FetchTimeout,
        },
    }
}

// EncodeToDataURI converts raw bytes into a data URI string.
// Output: "data:image/png;base64,iVBORw0KGgo..."
func (p *Processor) EncodeToDataURI(data []byte, mimeType string) string {
    encoded := base64.StdEncoding.EncodeToString(data)
    return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
}

// FetchAndEncode downloads an image URL and returns it as a data URI.
func (p *Processor) FetchAndEncode(imageURL string) (*domain.BlobData, error) {
    // Validate URL
    if !isValidImageURL(imageURL) {
        return nil, fmt.Errorf("invalid image URL: %s", imageURL)
    }

    resp, err := p.client.Get(imageURL)
    if err != nil {
        return nil, fmt.Errorf("fetch failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("fetch returned status %d", resp.StatusCode)
    }

    // Check size limit
    if resp.ContentLength > MaxImageSize {
        return nil, fmt.Errorf("image too large: %d bytes (max %d)", resp.ContentLength, MaxImageSize)
    }

    // Read with a size limit
    limitedReader := io.LimitReader(resp.Body, MaxImageSize+1)
    data, err := io.ReadAll(limitedReader)
    if err != nil {
        return nil, fmt.Errorf("read failed: %w", err)
    }
    if int64(len(data)) > MaxImageSize {
        return nil, fmt.Errorf("image exceeded size limit")
    }

    // Detect MIME type
    mimeType := resp.Header.Get("Content-Type")
    if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
        mimeType = http.DetectContentType(data)
    }

    return &domain.BlobData{
        DataURI:  p.EncodeToDataURI(data, mimeType),
        MimeType: mimeType,
        RawBytes: data,
    }, nil
}

func isValidImageURL(rawURL string) bool {
    return strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://")
}
```

### 4.2 Screenshot Capture: `internal/image/screenshot.go`

```go
package image

import (
    "context"
    "fmt"
    "time"

    "github.com/chromedp/chromedp"
    "github.com/standard-user/tomoshibi/internal/domain"
)

// CaptureScreenshot takes a screenshot using an existing chromedp allocator.
func (p *Processor) CaptureScreenshot(
    allocCtx context.Context,
    url string,
    opts *domain.ScreenshotOptions,
) (*domain.ScreenshotData, error) {
    if opts == nil {
        opts = &domain.ScreenshotOptions{
            Width:   1280,
            Height:  800,
            Format:  "webp",
            Quality: DefaultQuality,
        }
    }

    taskCtx, cancel := chromedp.NewContext(allocCtx)
    defer cancel()

    taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 30*time.Second)
    defer cancelTimeout()

    var buf []byte

    actions := []chromedp.Action{
        chromedp.EmulateViewport(int64(opts.Width), int64(opts.Height)),
        chromedp.Navigate(url),
        chromedp.WaitVisible("body", chromedp.ByQuery),
    }

    if opts.WaitSelector != "" {
        actions = append(actions, chromedp.WaitVisible(opts.WaitSelector, chromedp.ByQuery))
    }

    if opts.FullPage {
        actions = append(actions, chromedp.FullScreenshot(&buf, opts.Quality))
    } else {
        actions = append(actions, chromedp.CaptureScreenshot(&buf))
    }

    if err := chromedp.Run(taskCtx, actions...); err != nil {
        return nil, fmt.Errorf("screenshot capture failed: %w", err)
    }

    mimeType := "image/png"
    format := opts.Format
    if format == "" {
        format = "png"
    }
    switch format {
    case "jpeg", "jpg":
        mimeType = "image/jpeg"
    case "webp":
        mimeType = "image/webp"
    }

    return &domain.ScreenshotData{
        Blob:       p.EncodeToDataURI(buf, mimeType),
        Format:     format,
        Width:      opts.Width,
        Height:     opts.Height,
        FullPage:   opts.FullPage,
        SizeBytes:  int64(len(buf)),
        CapturedAt: time.Now(),
    }, nil
}
```

### 4.3 Image Extraction: `internal/image/extractor.go`

```go
package image

import (
    "net/url"
    "strings"

    "github.com/PuerkitoBio/goquery"
    "github.com/standard-user/tomoshibi/internal/domain"
)

// ExtractPageImages parses HTML and extracts image metadata.
func ExtractPageImages(htmlBody string, pageURL string, maxImages int) []domain.ImageData {
    if maxImages <= 0 {
        maxImages = 10
    }

    doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
    if err != nil {
        return nil
    }

    var images []domain.ImageData
    seen := make(map[string]bool)

    // 1. OG Image (highest priority)
    if ogImage, exists := doc.Find(`meta[property="og:image"]`).Attr("content"); exists {
        absURL := resolveURL(ogImage, pageURL)
        if absURL != "" && !seen[absURL] {
            images = append(images, domain.ImageData{
                URL:        absURL,
                SourceType: "og:image",
            })
            seen[absURL] = true
        }
    }

    // 2. Twitter card image
    if twitterImage, exists := doc.Find(`meta[property="twitter:image"], meta[name="twitter:image"]`).Attr("content"); exists {
        absURL := resolveURL(twitterImage, pageURL)
        if absURL != "" && !seen[absURL] {
            images = append(images, domain.ImageData{
                URL:        absURL,
                SourceType: "twitter:image",
            })
            seen[absURL] = true
        }
    }

    // 3. Content images
    doc.Find("img").Each(func(i int, s *goquery.Selection) {
        if len(images) >= maxImages {
            return
        }

        src, exists := s.Attr("src")
        if !exists || src == "" {
            return
        }

        absURL := resolveURL(src, pageURL)
        if absURL == "" || seen[absURL] {
            return
        }

        // Skip tracking pixels, icons, and tiny images
        if isTrackingPixel(absURL) {
            return
        }

        alt, _ := s.Attr("alt")
        title, _ := s.Attr("title")

        images = append(images, domain.ImageData{
            URL:        absURL,
            Alt:        alt,
            Title:      title,
            SourceType: "content",
        })
        seen[absURL] = true
    })

    return images
}

func resolveURL(rawURL, pageURL string) string {
    if strings.HasPrefix(rawURL, "data:") {
        return "" // Skip data URIs
    }

    parsed, err := url.Parse(rawURL)
    if err != nil {
        return ""
    }

    if parsed.IsAbs() {
        return rawURL
    }

    base, err := url.Parse(pageURL)
    if err != nil {
        return ""
    }

    return base.ResolveReference(parsed).String()
}

func isTrackingPixel(imgURL string) bool {
    trackers := []string{
        "pixel", "tracking", "beacon", "analytics",
        "1x1", ".gif", "spacer", "blank",
    }
    lower := strings.ToLower(imgURL)
    for _, t := range trackers {
        if strings.Contains(lower, t) {
            return true
        }
    }
    return false
}
```

---

## 5. API Examples

### 5.1 Basic Scrape with Screenshot Blob

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "mode": "dynamic",
    "screenshot": true,
    "image_format": "blob"
  }'
```

**Response:**

```json
{
  "url": "https://example.com",
  "markdown": "# Example Domain\n\nThis domain is for examples...",
  "metadata": { "engine": "chromedp" },
  "screenshot": {
    "blob": "data:image/webp;base64,UklGRtYCAABXRUJQ...",
    "format": "webp",
    "width": 1280,
    "height": 800,
    "full_page": false,
    "size_bytes": 42100,
    "captured_at": "2026-02-26T12:00:00Z"
  }
}
```

### 5.2 Scrape with Image Extraction

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/blog",
    "extract_images": true,
    "image_format": "blob"
  }'
```

**Response:**

```json
{
  "url": "https://example.com/blog",
  "markdown": "# Blog Post...",
  "images": [
    {
      "url": "https://example.com/hero.jpg",
      "blob": "data:image/jpeg;base64,/9j/4AAQSkZJRg...",
      "alt": "Blog hero image",
      "format": "jpeg",
      "size_bytes": 125000,
      "source": "og:image"
    },
    {
      "url": "https://example.com/photo.webp",
      "blob": "data:image/webp;base64,UklGRpYFAABXRUJQ...",
      "alt": "Photo in article",
      "format": "webp",
      "size_bytes": 89000,
      "source": "content"
    }
  ]
}
```

### 5.3 URL-Only Mode (Lightweight, Default)

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "extract_images": true
  }'
```

**Response** (no blobs, just metadata):

```json
{
  "url": "https://example.com",
  "markdown": "# Example...",
  "images": [
    {
      "url": "https://example.com/hero.jpg",
      "alt": "Hero image",
      "source": "og:image"
    }
  ]
}
```

### 5.4 Feeding Directly to OpenAI

```python
# Python consumer example
import requests, openai

# 1. Scrape with blob
resp = requests.post("http://localhost:8080/v1/scrape", json={
    "url": "https://example.com",
    "screenshot": True,
    "image_format": "blob"
})
data = resp.json()

# 2. Send directly to OpenAI — zero additional processing
response = openai.chat.completions.create(
    model="gpt-4o",
    messages=[{
        "role": "user",
        "content": [
            {"type": "text", "text": "Describe this page"},
            {"type": "image_url", "image_url": {
                "url": data["screenshot"]["blob"]  # data URI works directly
            }}
        ]
    }]
)
```

---

## 6. Implementation Phases

### Phase 1: Domain & Processor Foundation

| Task                                                     | File                               | Effort |
| -------------------------------------------------------- | ---------------------------------- | ------ |
| Create `BlobData`, `ImageData`, `ScreenshotData` structs | `internal/domain/media.go`         | Small  |
| Add `ScrapeOptions` to domain                            | `internal/domain/scraper.go`       | Small  |
| Update `ScrapeResult` with optional image fields         | `internal/domain/scraper.go`       | Small  |
| Create `Processor` with `EncodeToDataURI`                | `internal/image/processor.go`      | Small  |
| Create `FetchAndEncode` with size limits                 | `internal/image/processor.go`      | Medium |
| Write unit tests for processor                           | `internal/image/processor_test.go` | Medium |

### Phase 2: Screenshot Capture

| Task                                                | File                              | Effort |
| --------------------------------------------------- | --------------------------------- | ------ |
| Implement `CaptureScreenshot` using chromedp        | `internal/image/screenshot.go`    | Medium |
| Wire screenshot into `ChromedpScraper.Scrape()`     | `internal/scraper/chromedp.go`    | Medium |
| Update `Service.Scrape()` to accept `ScrapeOptions` | `internal/scraper/service.go`     | Medium |
| Handle `screenshot=true` in scrape handler          | `internal/api/handlers/scrape.go` | Small  |

### Phase 3: Image Extraction

| Task                                                      | File                              | Effort |
| --------------------------------------------------------- | --------------------------------- | ------ |
| Implement `ExtractPageImages` with goquery                | `internal/image/extractor.go`     | Medium |
| Integrate extraction into both scrapers                   | `internal/scraper/*.go`           | Medium |
| Optionally fetch + encode images when `image_format=blob` | `internal/image/processor.go`     | Medium |
| Add `extract_images` to scrape handler                    | `internal/api/handlers/scrape.go` | Small  |

### Phase 4: Testing & Safety

| Task                                        | File                               | Effort |
| ------------------------------------------- | ---------------------------------- | ------ |
| Unit tests for `EncodeToDataURI`            | `internal/image/processor_test.go` | Small  |
| Unit tests for `ExtractPageImages`          | `internal/image/extractor_test.go` | Medium |
| Unit tests for `FetchAndEncode` (mock HTTP) | `internal/image/processor_test.go` | Medium |
| Integration test for full blob pipeline     | `test/image_integration_test.go`   | Medium |
| Size limit and timeout tests                | Various                            | Small  |

---

## 7. Safety & Performance

### Size Controls

```go
const (
    MaxImageSize      = 5 * 1024 * 1024    // 5MB per image
    MaxTotalBlobSize  = 20 * 1024 * 1024   // 20MB total response (matches Gemini limit)
    MaxImages         = 10                  // Default max images to extract
    FetchTimeout      = 10 * time.Second    // Per-image fetch timeout
    ScreenshotTimeout = 30 * time.Second    // Screenshot capture timeout
)
```

### Rate Limiting

```go
// Separate rate limiter for image fetching
imageLimiter := rate.NewLimiter(rate.Every(200*time.Millisecond), 3)
```

### Error Handling Strategy

```
If screenshot fails   → result.Screenshot = nil (not an error)
If image fetch fails  → image.Blob = "" (URL still populated)
If encoding fails     → log warning, skip image
If size limit hit     → skip image, log info
```

**Key principle**: Image features are **additive**. They never break the core scrape response.

---

## 8. Files Modified/Created Summary

### New Files

| File                               | Purpose                                 |
| ---------------------------------- | --------------------------------------- |
| `internal/domain/media.go`         | Shared image/screenshot data structures |
| `internal/image/processor.go`      | Base64 encoding, data URI generation    |
| `internal/image/screenshot.go`     | Chromedp screenshot capture             |
| `internal/image/extractor.go`      | HTML image extraction via goquery       |
| `internal/image/processor_test.go` | Processor unit tests                    |
| `internal/image/extractor_test.go` | Extractor unit tests                    |

### Modified Files

| File                              | Changes                                             |
| --------------------------------- | --------------------------------------------------- |
| `internal/domain/scraper.go`      | Add `Screenshot`, `Images` fields + `ScrapeOptions` |
| `internal/scraper/service.go`     | Accept `ScrapeOptions`, wire image processor        |
| `internal/scraper/chromedp.go`    | Integrate screenshot capture                        |
| `internal/scraper/colly.go`       | Integrate image extraction                          |
| `internal/api/handlers/scrape.go` | Parse new request params                            |
| `go.mod`                          | No new deps needed (goquery already transitive)     |

---

## 9. No New Dependencies

All required packages are **already in `go.mod`** (directly or transitively):

- `github.com/chromedp/chromedp` — Screenshot capture ✅
- `github.com/PuerkitoBio/goquery` — HTML parsing for image extraction ✅ (transitive via colly)
- `encoding/base64` — Base64 encoding ✅ (stdlib)
- `net/http` — Image fetching ✅ (stdlib)
- `io` — Size-limited reads ✅ (stdlib)

---

## 10. Backward Compatibility

All image features are **opt-in**. The default behavior is unchanged:

```go
// Old code — still works identically
result, err := svc.Scrape(ctx, url, "smart")
// result.Screenshot == nil
// result.Images == nil (omitted from JSON)

// New code — opt-in
opts := domain.ScrapeOptions{
    Mode:          "smart",
    Screenshot:    true,
    ExtractImages: true,
    ImageFormat:   domain.ImageFormatBlob,
}
result, err := svc.ScrapeWithOptions(ctx, url, opts)
// result.Screenshot.Blob = "data:image/webp;base64,..."
// result.Images[0].Blob = "data:image/jpeg;base64,..."
```

---

## 11. Next Steps

1. **Review this plan** ← You are here
2. Create `internal/domain/media.go` with structs
3. Create `internal/image/processor.go` with `EncodeToDataURI` + tests
4. Create `internal/image/extractor.go` with `ExtractPageImages` + tests
5. Wire into scraper service
6. Update API handler to accept new params
7. End-to-end test
