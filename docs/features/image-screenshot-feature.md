# Image and Screenshot Feature Implementation

**Document Location:** `docs/features/image-screenshot-feature.md`
**Related:** [Image Blob Transport](./image-blob-transport.md) | [Documentation Index](../guides/INDEX.md)
**Status:** ✅ **Implemented**

---

## Overview

This document outlines the implementation of image and screenshot capture capabilities in Tomoshibi. Users can now toggle `screenshot` and `images` flags in both the **Synchronous Scrape API** and **Asynchronous Crawl API**.

### Current Implementation

- **API Handlers** (`internal/api/handlers`): `scrape` and `crawl` endpoints support `screenshot` and `images` parameters.
- **Scrape Service** (`internal/scraper`): Passes options down to the underlying engine.
- **Chromedp Engine** (`internal/scraper/chromedp.go`): Captures full-page screenshots as base64-encoded JPEGs.
- **Asynq Worker** (`internal/worker`): Propagates flags through the background task system.

---

## 1. Architecture Overview

### New Service Layer Structure

We will move image-related data structures to the `internal/domain` layer to ensure they are accessible by both the Scraper and Search services (avoiding circular dependencies).

```
tomoshibi/
├── internal/
│   ├── domain/
│   │   ├── scraper.go          (EXISTING - Update ScrapeResult)
│   │   └── media.go            (NEW - Image/Screenshot structs)
│   ├── scraper/
│   │   ├── chromedp.go         (EXISTING - Add screenshot logic)
│   │   ├── colly.go            (EXISTING - Add image extraction)
│   │   └── service.go          (EXISTING)
│   ├── search/
│   │   ├── service.go          (EXISTING - Helper usage)
│   ├── image/                  (NEW - Optional helper service)
│   │   ├── processor.go        (Image resizing/optimization)
│   │   └── service.go          (Shared logic)
└── docs/
    └── features/
        └── image-screenshot-feature.md (THIS FILE)
```

### Architecture Decision

We place the core data models (`ImageData`, `ScreenshotData`) in `internal/domain` so they can be used by `ScrapeResult` (in `domain`) and optionally `SearchResult` (in `search`). The extraction logic resides in the respective scrapers (`chromedp`/`colly`), while shared processing logic (resizing, uploading) goes into `internal/image`.

---

## 2. Data Structures

### 2.1 New Domain Models

**File:** `internal/domain/media.go` (New File)

```go
package domain

import "time"

// ImageData represents a single image found on a page
type ImageData struct {
    URL        string `json:"url"`
    Title      string `json:"title,omitempty"`
    Alt        string `json:"alt,omitempty"`
    Width      int    `json:"width,omitempty"`
    Height     int    `json:"height,omitempty"`
    Format     string `json:"format,omitempty"` // "png", "jpeg", "webp"
    Size       int64  `json:"size,omitempty"`   // bytes
    SourceType string `json:"source_type"`      // "og:image", "favicon", "content"
}

// ScreenshotData represents a captured website screenshot
type ScreenshotData struct {
    Data       []byte    `json:"data,omitempty"` // Base64 encoded in JSON
    URL        string    `json:"url,omitempty"`  // If stored externally
    Format     string    `json:"format"`         // "png", "jpeg"
    Width      int       `json:"width"`
    Height     int       `json:"height"`
    FullPage   bool      `json:"full_page"`
    CapturedAt time.Time `json:"captured_at"`
    Error      string    `json:"error,omitempty"`
}

// ScreenshotOptions configures screenshot capture behavior
type ScreenshotOptions struct {
    Width          int           // viewport width in pixels
    Height         int           // viewport height in pixels
    FullPage       bool          // capture entire page vs viewport
    WaitSelector   string        // CSS selector to wait for before capturing
    Timeout        time.Duration // max time to wait for page load
    Format         string        // "png" or "jpeg"
    Quality        int           // 1-100 for jpeg quality
    BlockAds       bool          // block ad scripts
}
```

### 2.2 Updated ScrapeResult

**File:** `internal/domain/scraper.go`

**Current:**

```go
type ScrapeResult struct {
	URL      string            `json:"url"`
	Markdown string            `json:"markdown"`
	HTML     string            `json:"html,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
```

**Modified:**

```go
type ScrapeResult struct {
	URL        string            `json:"url"`
	Markdown   string            `json:"markdown"`
	HTML       string            `json:"html,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
    // NEW FIELDS
    Screenshot *ScreenshotData   `json:"screenshot,omitempty"`
    Images     []ImageData       `json:"images,omitempty"`
}
```

### 2.3 ScrapeOptions & SearchOptions

We need to update **SearchOptions** in `internal/search` and create/update **ScrapeOptions** in `internal/domain` (or passed as args).

**File:** `internal/search/service.go` (SearchOptions)

```go
type SearchOptions struct {
    // ... existing fields ...
    IncludeImages    bool
    IncludeScreenshot bool
}
```

**File:** `internal/domain/scraper.go` (ScrapeOptions)

```go
type ScrapeOptions struct {
    URL            string
    Mode           string // "static", "dynamic", "smart"

    // OPTIONAL FEATURES (Resource Control)
    Screenshot     bool   // If true, loads images and captures screenshot (Heavier)
    ExtractImages  bool   // If true, extracts image metadata (Lightweight)

    ScreenshotOpts *ScreenshotOptions
}
```

---

## 3. Service Strategy

### 3.1 Chromedp Scraper (Screenshots)

**File:** `internal/scraper/chromedp.go`

The `chromedp` scraper is best suited for screenshots. We will implement `CaptureScreenshot` logic within its `Scrape` method.

#### Resource Optimization Strategy

To ensure efficiency and optionality as requested:

1.  **Optional Screenshots**: Screenshots are only captured if `ScrapeOptions.Screenshot` is true.
2.  **Bandwidth Saving**: If `ScrapeOptions.Screenshot` is **false**, we will explicitly block image resources to save bandwidth and speed up loading.
    - Use `network.SetBlockedURLs` with patterns like `*.png`, `*.jpg`, `*.gif`.
3.  **Lazy Extraction**: `ImageData` extraction in dynamic mode will parse the DOM (which exists even if images are blocked) rather than downloading the image content.

```go
// Pseudo-code implementation logic for Scrape method
func (s *ChromedpScraper) Scrape(ctx context.Context, url string, opts ScrapeOptions) (*domain.ScrapeResult, error) {

    // 1. Setup Tasks
    tasks := []chromedp.Action{}

    // RESOURCE OPTIMIZATION: Block images if we don't need a screenshot
    if !opts.Screenshot {
        tasks = append(tasks, network.Enable(), network.SetBlockedURLs([]string{
            "*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "image/*",
        }))
    }

    // 2. Navigation
    tasks = append(tasks,
        chromedp.Navigate(url),
        // ... wait for load ...
    )

    // 3. Screenshot Capture (Optional)
    if opts.Screenshot {
        // ... implementation of screenshot capture ...
        // Note: Images must NOT be blocked here for screenshot to look correct
    }

    // ... execute tasks ...
}
```

### 3.2 Colly Scraper (Image Extraction)

**File:** `internal/scraper/colly.go`

Colly will be enhanced to parse `<img>` tags and populate `ScrapeResult.Images`.

---

## 4. Implementation Strategy

### Phase 1: Domain & Foundation

1.  **Domain Models**: Create `internal/domain/media.go` and update `ScrapeResult` in `scraper.go`.
2.  **Image Service**: Create `internal/image` for image processing/resizing utils (using `disintegration/imaging`).

### Phase 2: Scraper Implementation (Core)

1.  **Chromedp**: Implement screenshot capture in `internal/scraper/chromedp.go`.
    - Add `CaptureScreenshot` logic.
    - Handle `ScreenshotOptions` (size, format).
    - **Optimization**: Implement `network.SetBlockedURLs` to block images when `Screenshot=false` to save bandwidth.
2.  **Colly**: Implement image extraction in `internal/scraper/colly.go`.
    - Extract `src` from `<img>` tags (Lightweight, no download required).
    - Basic filtering (dimensions, tracking pixels).
3.  **Service**: Update `internal/scraper/service.go` `Scrape` method to accept options and pass them down.

### Phase 3: Integration & Search

1.  Update `Search` service to optionally utilize the Scraper to fetch screenshots for top results (if requested).
2.  Ensure backward compatibility for existing consumers.

---

## 5. Key Dependencies

### Required Go Packages

```bash
# Image processing (pure Go, no C dependencies)
go get github.com/disintegration/imaging@latest

# Headless browser automation
go get github.com/chromedp/chromedp@latest

# Context management (already imported)
# golang.org/x/time/rate (already imported)
```

### Why These Choices?

| Package                    | Reason                                                 | Alternative                          |
| -------------------------- | ------------------------------------------------------ | ------------------------------------ |
| **disintegration/imaging** | Pure Go, no native deps, simple API                    | bimg (needs libvips), graphicsmagick |
| **chromedp/chromedp**      | Official Chrome DevTools Protocol, actively maintained | playwright-go, puppeteer-go          |

---

## 6. Error Handling & Safety

### Preventing Breakage

```go
// 1. Optional Features - All image operations are opt-in
if opts.IncludeImages {
    // only execute if explicitly requested
}

// 2. Graceful Degradation - Missing images don't fail search
results, err := s.Search(ctx, opts) // succeeds even if image fetch fails
// individual images may be nil, but search results are complete

// 3. Timeout Protection
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
// prevents hanging requests

// 4. Rate Limiting - Apply separate limits for images
imageLimiter := rate.NewLimiter(rate.Every(500*time.Millisecond), 1)

// 5. Validation - Check URLs before processing
if !isValidImageURL(url) {
    return nil, fmt.Errorf("invalid image URL")
}
```

### Testing Strategy

**File:** `internal/image/service_test.go`

```go
func TestImageService_FetchImage_InvalidURL(t *testing.T) {
    // Test with malformed URLs
}

func TestImageService_Screenshot_Timeout(t *testing.T) {
    // Test timeout handling
}

func TestSearchService_BackwardCompatibility(t *testing.T) {
    // Ensure old SearchOptions still work
}

func TestResultJSON_OmitEmptyImages(t *testing.T) {
    // Verify images field omitted when empty
}
```

---

## 4. Configuration

Image features are controlled per-request via API parameters — there are no dedicated env vars for enabling/disabling them globally.

| API Parameter       | Type    | Default | Description                                      |
| ------------------- | ------- | ------- | ------------------------------------------------ |
| `screenshot`        | boolean | `false` | Capture full-page base64 JPEG screenshot         |
| `images`            | boolean | `false` | Extract images from the page                     |
| `image_format`      | string  | `"url"` | `"url"` (metadata) or `"blob"` (base64 data URI) |
| `max_images`        | int     | `10`    | Maximum images to extract                        |
| `max_image_size_kb` | int     | `5120`  | Max individual image size (5MB default)          |

Hardcoded limits in `internal/image/processor.go`:

- `MaxImageSize = 5 * 1024 * 1024` (5MB per image)
- `FetchTimeout = 10 * time.Second`
- `DefaultQuality = 80` (JPEG/WebP)

> The image pipeline does not write to disk — everything stays in memory and is passed as base64 data URIs in the JSON response.

---

## 8. Migration Path

### For JavaScript Developers

Since you're coming from a JS background, think of this like this:

```javascript
// BEFORE (like current Tomoshibi)
const results = await search("golang tutorials");
// Returns: [ { title, url, description, ... } ]

// AFTER (with image features)
const results = await search("golang tutorials", {
  includeImages: true,
  includeScreenshot: true,
});
// Returns: [ {
//   title, url, description,
//   images: [ { url, width, height, ... } ],
//   screenshot: { data, width, height, ... }
// } ]

// Backward compatible - old code works:
const results = await search("golang tutorials");
// Still works, images not fetched
```

### Go Equivalent

```go
// BEFORE
results, _, err := s.Search(ctx, SearchOptions{Query: "golang"})

// AFTER - with images (opt-in)
results, _, err := s.Search(ctx, SearchOptions{
    Query:           "golang",
    IncludeImages:   true,
    IncludeScreenshot: true,
})

// BACKWARD COMPATIBLE - old code unaffected
results, _, err := s.Search(ctx, SearchOptions{Query: "golang"})
// IncludeImages defaults to false
```

---

## ✅ Implementation Status

All phases have been implemented. Here's the current state:

### ✅ Phase 1: Domain & Foundation (DONE)

- `internal/domain/media.go` — Contains `ImageData`, `ScreenshotData`, `BlobData`, `ScreenshotOptions`, `ImageTransportFormat`
- `internal/domain/scraper.go` — `ScrapeResult` has `Screenshot` and `Images` fields; `ScrapeOptions` has all imaging flags

### ✅ Phase 2: Scraper Implementation (DONE)

- `internal/scraper/chromedp.go` — Captures full-page base64 JPEG screenshots when `opts.Screenshot` is true
- `internal/scraper/colly.go` — Returns HTML for centralized image extraction in service layer
- `internal/scraper/service.go` — Passes `ScrapeOptions` through; forces dynamic mode when screenshot is requested; runs `image.ExtractPageImages()` on returned HTML when `opts.Images` is true; runs `image.Processor.FetchAndEncode()` for blob encoding

### ✅ Phase 3: Scrape Service Integration (DONE)

Image extraction is now wired into `internal/scraper/service.go`:

- After each scraper returns HTML, if `opts.Images` is true, `image.ExtractPageImages()` populates `result.Images`
- If `opts.ImageFormat` is `"blob"`, `image.Processor.FetchAndEncode()` fetches and encodes each image to base64 data URIs
- All image fields (`ImageFormat`, `MaxImages`, `MaxImageSizeKB`) are exposed in the API (`ScrapeRequest`) and worker tasks (`ScrapePayload`, `CrawlPayload`)
- Screenshots and image extraction work independently — screenshots via chromedp, image extraction via service layer

### ✅ Phase 4: Infrastructure (DONE)

- Size limits enforced in `internal/image/processor.go` (5MB per image, 10s fetch timeout)
- Dockerfile already includes Chromium for headless screenshots
- `install_browser.sh` handles Chrome installation in CI/deployment

---

## 10. Files Modified/Created Summary

### Modified Files

| File                           | Changes                                         |
| ------------------------------ | ----------------------------------------------- |
| `internal/domain/scraper.go`   | Add `Screenshot` and `Images` to `ScrapeResult` |
| `internal/scraper/service.go`  | Update method signatures / options handling     |
| `internal/scraper/chromedp.go` | Implement screenshot logic                      |
| `internal/scraper/colly.go`    | Implement image extraction                      |
| `internal/search/service.go`   | (Optional) Add metadata fields                  |
| `go.mod`                       | Add dependencies                                |

### New Files

| File                              | Purpose                                 |
| --------------------------------- | --------------------------------------- |
| `internal/domain/media.go`        | Shared image/screenshot data structures |
| `internal/image/processor.go`     | Image manipulation utilities            |
| `internal/config/image_config.go` | Configuration                           |

---

## 11. Security Considerations

### URL Validation

```go
// Always validate URLs before fetching
func isValidImageURL(rawURL string) bool {
    u, err := url.Parse(rawURL)
    if err != nil {
        return false
    }
    // Only http/https
    return u.Scheme == "http" || u.Scheme == "https"
}
```

### Size Limits

```go
// Prevent downloading huge files
const MaxImageSize = 5 * 1024 * 1024 // 5MB
if resp.ContentLength > MaxImageSize {
    return fmt.Errorf("image too large")
}
```

### Timeout Protection

```go
// Always use context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

---

## 12. References

### Go Documentation

- [Context Package](https://pkg.go.dev/context) - Request-scoped values and cancellation
- [Go Image Package](https://go.dev/blog/image) - Image manipulation basics
- [HTTP Client](https://pkg.go.dev/net/http) - Making HTTP requests

### External Libraries

- [disintegration/imaging](https://pkg.go.dev/github.com/disintegration/imaging) - Image processing
- [chromedp/chromedp](https://pkg.go.dev/github.com/chromedp/chromedp) - Browser automation
- [Rate Limiting in Go](https://pkg.go.dev/golang.org/x/time/rate) - Request throttling

---

## 13. Next Steps

1. **Review this plan** with the team
2. **Create feature branch**: `feature/image-screenshot-service`
3. **Start Phase 1** with foundation work
4. **Write tests first** (TDD approach)
5. **Gradually integrate** each component
6. **Performance test** before Phase 3 completion

---

**Document Version:** 1.0  
**Last Updated:** January 26, 2026  
**Status:** ✅ **Implemented** (see image-blob-transport.md for current architecture)
