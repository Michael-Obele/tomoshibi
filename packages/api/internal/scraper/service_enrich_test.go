package scraper

import (
	"bytes"
	"context"
	"fmt"
	stdimg "image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// enrichHTML wraps body in a document with enough filler text that the SPA
// heuristics never fire — these tests are about post-scrape enrichment, not
// engine selection.
func enrichHTML(body string) string {
	return "<html><body>" + body + makeStaticHTML() + "</body></html>"
}

// scrapeWith runs a static scrape over the given HTML and markdown. Every
// enrichment step below hangs off the static path, so this removes the engine
// setup from each test.
func scrapeWith(t *testing.T, html, markdown string, meta map[string]string, opts domain.ScrapeOptions) *domain.ScrapeResult {
	t.Helper()

	if meta == nil {
		meta = map[string]string{"engine": "colly"}
	}
	colly := &mockScraper{result: &domain.ScrapeResult{
		URL:      "https://example.com",
		HTML:     html,
		Markdown: markdown,
		Metadata: meta,
	}}
	svc := NewService(colly, nil, nil)

	result, err := svc.Scrape(context.Background(), "https://example.com", "static", opts)
	if err != nil {
		t.Fatalf("Scrape returned an error: %v", err)
	}
	return result
}

// TestScrape_ExtractSchema covers the deterministic extraction step and the
// two guards that skip it.
func TestScrape_ExtractSchema(t *testing.T) {
	schema := map[string]domain.ExtractField{
		"headline": {Selector: "h1"},
		"tags":     {Selector: ".tag", Multiple: true},
		"missing":  {Selector: ".nope"},
	}

	t.Run("populates Extracted", func(t *testing.T) {
		result := scrapeWith(t,
			enrichHTML(`<h1>Real Headline</h1><span class="tag">go</span><span class="tag">web</span>`),
			"# Real Headline", nil,
			domain.ScrapeOptions{ExtractSchema: schema})

		if result.Extracted == nil {
			t.Fatal("Extracted is nil")
		}
		if got := result.Extracted["headline"]; got != "Real Headline" {
			t.Errorf("headline = %v, want %q", got, "Real Headline")
		}
		tags, ok := result.Extracted["tags"].([]string)
		if !ok {
			t.Fatalf("tags = %T, want []string", result.Extracted["tags"])
		}
		if len(tags) != 2 || tags[0] != "go" || tags[1] != "web" {
			t.Errorf("tags = %v, want [go web]", tags)
		}
		// A selector that matches nothing omits the field rather than
		// recording an empty value, so callers can tell "absent" from "blank".
		if _, present := result.Extracted["missing"]; present {
			t.Error("a non-matching selector produced a field")
		}
	})

	t.Run("skipped without a schema", func(t *testing.T) {
		result := scrapeWith(t, enrichHTML("<h1>Headline</h1>"), "# Headline", nil, domain.ScrapeOptions{})
		if result.Extracted != nil {
			t.Errorf("Extracted = %v, want nil when no schema was requested", result.Extracted)
		}
	})

	t.Run("skipped when the engine returned no HTML", func(t *testing.T) {
		// Markdown-only results (some engines strip HTML) have nothing to run
		// selectors against; extraction must be skipped, not attempted.
		result := scrapeWith(t, "", "# Headline", nil, domain.ScrapeOptions{ExtractSchema: schema})
		if result.Extracted != nil {
			t.Errorf("Extracted = %v, want nil when there is no HTML", result.Extracted)
		}
	})
}

// TestScrape_Summary covers the extractive summary step, including the
// readability excerpt taking precedence over the markdown body.
func TestScrape_Summary(t *testing.T) {
	const markdown = "Go is a statically typed language. It compiles quickly to machine code. " +
		"Goroutines make concurrency cheap and readable for most programs."

	t.Run("summarizes the markdown", func(t *testing.T) {
		result := scrapeWith(t, enrichHTML(""), markdown, nil, domain.ScrapeOptions{
			Summary:          true,
			SummarySentences: 2,
		})
		if result.Summary == "" {
			t.Fatal("Summary is empty")
		}
	})

	t.Run("prefers the readability excerpt", func(t *testing.T) {
		result := scrapeWith(t, enrichHTML(""), markdown,
			map[string]string{"engine": "colly", "excerpt": "A hand-written excerpt."},
			domain.ScrapeOptions{Summary: true})

		if result.Summary != "A hand-written excerpt." {
			t.Errorf("Summary = %q, want the excerpt", result.Summary)
		}
	})

	t.Run("skipped when not requested", func(t *testing.T) {
		result := scrapeWith(t, enrichHTML(""), markdown, nil, domain.ScrapeOptions{})
		if result.Summary != "" {
			t.Errorf("Summary = %q, want empty", result.Summary)
		}
	})
}

// TestScrape_RedactPII asserts redaction reaches both the markdown and the
// summary. The summary is derived from the markdown, so redacting only one of
// them would leak the same address back out through the other.
func TestScrape_RedactPII(t *testing.T) {
	const markdown = "Contact us at sales@example.com for details. " +
		"Our support desk answers on 555-123-4567 during business hours."

	result := scrapeWith(t, enrichHTML(""), markdown, nil, domain.ScrapeOptions{
		Summary:   true,
		RedactPII: true,
	})

	if strings.Contains(result.Markdown, "sales@example.com") {
		t.Errorf("Markdown still contains the email: %q", result.Markdown)
	}
	if !strings.Contains(result.Markdown, "[EMAIL]") {
		t.Errorf("Markdown = %q, want the [EMAIL] token", result.Markdown)
	}
	if result.Summary == "" {
		t.Fatal("Summary is empty, so the summary redaction path was not exercised")
	}
	if strings.Contains(result.Summary, "sales@example.com") {
		t.Errorf("Summary still contains the email: %q", result.Summary)
	}
}

// pngBytes returns a valid w×h PNG. Building it with the standard encoder
// keeps it decodable by image.Decode, which the re-encode path requires.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()

	img := stdimg.NewRGBA(stdimg.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

// TestScrape_MaxImagesDefault pins the default cap. Without it a page with
// hundreds of images would be fetched in full.
func TestScrape_MaxImagesDefault(t *testing.T) {
	var markup strings.Builder
	for i := 0; i < 25; i++ {
		markup.WriteString(fmt.Sprintf(`<img src="https://cdn.example.com/%c.png" alt="x">`, 'a'+i))
	}
	html := enrichHTML(markup.String())

	tests := []struct {
		name      string
		maxImages int
		want      int
	}{
		{name: "zero defaults to 10", maxImages: 0, want: 10},
		{name: "negative also defaults to 10", maxImages: -1, want: 10},
		{name: "an explicit cap is honoured", maxImages: 3, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrapeWith(t, html, "# Page", nil, domain.ScrapeOptions{
				Images:      true,
				ImageFormat: domain.ImageFormatURL,
				MaxImages:   tt.maxImages,
			})
			if len(result.Images) != tt.want {
				t.Errorf("got %d images, want %d", len(result.Images), tt.want)
			}
			// URL format must not fetch anything.
			for i, img := range result.Images {
				if img.Blob != "" {
					t.Errorf("image %d has a blob in url format", i)
				}
			}
		})
	}
}

// TestScrape_ImageProcess covers the optional re-encode pass over fetched
// blobs: the success path, and a failure that must be skipped rather than
// failing the whole scrape.
func TestScrape_ImageProcess(t *testing.T) {
	good := pngBytes(t, 40, 20)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if strings.HasPrefix(r.URL.Path, "/broken") {
			// Claims to be a PNG but is not one. The fetch succeeds (the
			// header is taken at face value) and the decode fails later,
			// which is exactly the case the re-encode loop has to survive.
			_, _ = w.Write([]byte("this is not a png"))
			return
		}
		_, _ = w.Write(good)
	}))
	defer srv.Close()

	t.Run("re-encodes fetched blobs", func(t *testing.T) {
		result := scrapeWith(t,
			enrichHTML(`<img src="`+srv.URL+`/a.png">`),
			"# Page", nil,
			domain.ScrapeOptions{
				Images:       true,
				ImageFormat:  domain.ImageFormatBlob,
				ImageProcess: &domain.ImageProcessOptions{Format: "jpeg", MaxWidth: 10, Quality: 60},
			})

		if len(result.Images) != 1 {
			t.Fatalf("got %d images, want 1", len(result.Images))
		}
		img := result.Images[0]
		if img.Format != "jpeg" {
			t.Errorf("Format = %q, want jpeg", img.Format)
		}
		if !strings.HasPrefix(img.Blob, "data:image/jpeg;base64,") {
			t.Errorf("Blob does not carry the re-encoded mime type: %.40q", img.Blob)
		}
		if img.SizeBytes <= 0 {
			t.Errorf("SizeBytes = %d, want the re-encoded length", img.SizeBytes)
		}
	})

	t.Run("a failed re-encode leaves the original blob", func(t *testing.T) {
		result := scrapeWith(t,
			enrichHTML(`<img src="`+srv.URL+`/broken.png">`),
			"# Page", nil,
			domain.ScrapeOptions{
				Images:       true,
				ImageFormat:  domain.ImageFormatBlob,
				ImageProcess: &domain.ImageProcessOptions{Format: "jpeg"},
			})

		if len(result.Images) != 1 {
			t.Fatalf("got %d images, want 1", len(result.Images))
		}
		// The undecodable payload is still returned as fetched: enrichment
		// failing must not cost the caller the image.
		if !strings.HasPrefix(result.Images[0].Blob, "data:image/png;base64,") {
			t.Errorf("Blob = %.40q, want the original fetched data URI", result.Images[0].Blob)
		}
	})
}

// TestScrape_ImageFetchFailuresAreSkipped asserts a per-image fetch failure
// is logged and dropped rather than failing the scrape. The page content is
// the product; the images are a bonus.
func TestScrape_ImageFetchFailuresAreSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes(t, 4, 4))
	}))
	defer srv.Close()

	result := scrapeWith(t,
		enrichHTML(`<img src="`+srv.URL+`/missing.png"><img src="`+srv.URL+`/ok.png">`),
		"# Page", nil,
		domain.ScrapeOptions{Images: true, ImageFormat: domain.ImageFormatBlob})

	if len(result.Images) != 2 {
		t.Fatalf("got %d images, want both entries retained", len(result.Images))
	}
	var withBlob int
	for _, img := range result.Images {
		if img.Blob != "" {
			withBlob++
		}
	}
	if withBlob != 1 {
		t.Errorf("%d images carry a blob, want exactly the one that fetched", withBlob)
	}
}

// TestScrape_ImageFetchAbortsOnCancellation covers the one image error that
// is not swallowed. A cancelled context means the caller has gone away, so
// continuing to fetch is pointless; the errgroup propagates it instead.
func TestScrape_ImageFetchAbortsOnCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	colly := &mockScraper{result: &domain.ScrapeResult{
		URL:      "https://example.com",
		HTML:     enrichHTML(`<img src="` + srv.URL + `/a.png">`),
		Markdown: "# Page",
		Metadata: map[string]string{"engine": "colly"},
	}}
	svc := NewService(colly, nil, nil)

	// Cancelled up front so the guard is evaluated deterministically. The
	// image client does not take a context, so the fetch itself still runs
	// and fails on the 500 — what is being tested is that the failure is then
	// classified as cancellation rather than a skippable per-image error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Scrape(ctx, "https://example.com", "static", domain.ScrapeOptions{
		Images:      true,
		ImageFormat: domain.ImageFormatBlob,
	})
	if err == nil {
		t.Fatal("expected the cancelled fetch to fail the scrape, got nil")
	}
	if !strings.Contains(err.Error(), "image fetch aborted") {
		t.Errorf("error = %v, want it to mention the aborted fetch", err)
	}
}
