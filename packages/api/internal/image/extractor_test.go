package image

import (
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestExtractPageImages_OGImage(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://example.com/og-hero.jpg">
	</head><body><p>Content</p></body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 1 {
		t.Fatalf("Expected 1 image, got %d", len(images))
	}

	if images[0].URL != "https://example.com/og-hero.jpg" {
		t.Errorf("URL mismatch: got %q", images[0].URL)
	}

	if images[0].SourceType != "og:image" {
		t.Errorf("Source type mismatch: got %q", images[0].SourceType)
	}
}

func TestExtractPageImages_TwitterCard(t *testing.T) {
	html := `<html><head>
		<meta name="twitter:image" content="https://example.com/twitter-card.jpg">
	</head><body></body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 1 {
		t.Fatalf("Expected 1 image, got %d", len(images))
	}

	if images[0].SourceType != "twitter:image" {
		t.Errorf("Source type mismatch: got %q", images[0].SourceType)
	}
}

func TestExtractPageImages_ContentImages(t *testing.T) {
	html := `<html><body>
		<img src="https://example.com/photo1.jpg" alt="Photo 1" title="First Photo">
		<img src="https://example.com/photo2.png" alt="Photo 2">
		<img src="https://example.com/photo3.webp">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 3 {
		t.Fatalf("Expected 3 images, got %d", len(images))
	}

	if images[0].Alt != "Photo 1" {
		t.Errorf("Alt mismatch: got %q", images[0].Alt)
	}
	if images[0].Title != "First Photo" {
		t.Errorf("Title mismatch: got %q", images[0].Title)
	}
	if images[0].SourceType != "content" {
		t.Errorf("Source type mismatch: got %q", images[0].SourceType)
	}
}

func TestExtractPageImages_RelativeURLs(t *testing.T) {
	html := `<html><body>
		<img src="/images/photo.jpg" alt="Relative">
		<img src="assets/icon.png" alt="Relative path">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com/blog/post", 10)

	if len(images) != 2 {
		t.Fatalf("Expected 2 images, got %d", len(images))
	}

	if images[0].URL != "https://example.com/images/photo.jpg" {
		t.Errorf("Resolved URL mismatch: got %q", images[0].URL)
	}

	if images[1].URL != "https://example.com/blog/assets/icon.png" {
		t.Errorf("Resolved URL mismatch: got %q", images[1].URL)
	}
}

func TestExtractPageImages_DataURIs_Skipped(t *testing.T) {
	html := `<html><body>
		<img src="data:image/png;base64,iVBORw0KGgo=" alt="Data URI">
		<img src="https://example.com/real.jpg" alt="Real image">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 1 {
		t.Fatalf("Expected 1 image (data URIs skipped), got %d", len(images))
	}

	if images[0].URL != "https://example.com/real.jpg" {
		t.Errorf("URL mismatch: got %q", images[0].URL)
	}
}

func TestExtractPageImages_TrackingPixels_Skipped(t *testing.T) {
	html := `<html><body>
		<img src="https://example.com/tracking-pixel.gif" alt="">
		<img src="https://example.com/analytics/beacon.png" alt="">
		<img src="https://example.com/real-photo.jpg" alt="Real">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 1 {
		t.Fatalf("Expected 1 image (trackers skipped), got %d", len(images))
	}

	if images[0].URL != "https://example.com/real-photo.jpg" {
		t.Errorf("URL mismatch: got %q", images[0].URL)
	}
}

func TestExtractPageImages_MaxLimit(t *testing.T) {
	html := `<html><body>
		<img src="https://example.com/1.jpg">
		<img src="https://example.com/2.jpg">
		<img src="https://example.com/3.jpg">
		<img src="https://example.com/4.jpg">
		<img src="https://example.com/5.jpg">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com", 3)

	if len(images) != 3 {
		t.Errorf("Expected max 3 images, got %d", len(images))
	}
}

func TestExtractPageImages_Deduplication(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://example.com/hero.jpg">
	</head><body>
		<img src="https://example.com/hero.jpg" alt="Same as OG">
		<img src="https://example.com/other.jpg" alt="Different">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 2 {
		t.Errorf("Expected 2 unique images, got %d", len(images))
	}
}

func TestExtractPageImages_EmptyHTML(t *testing.T) {
	images := ExtractPageImages("", "https://example.com", 10)

	if len(images) != 0 {
		t.Errorf("Expected 0 images for empty HTML, got %d", len(images))
	}
}

func TestExtractPageImages_NoImages(t *testing.T) {
	html := `<html><body><p>No images here</p></body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 0 {
		t.Errorf("Expected 0 images, got %d", len(images))
	}
}

func TestExtractPageImages_EmptySrc(t *testing.T) {
	html := `<html><body>
		<img src="" alt="Empty src">
		<img alt="No src at all">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 0 {
		t.Errorf("Expected 0 images for empty/missing src, got %d", len(images))
	}
}

// --- v2: srcset, lazy-load, picture, optimizer, ranking ---

func TestPickFromSrcset_PicksLargestWidth(t *testing.T) {
	srcset := "a-480w.jpg 480w, a-800w.jpg 800w, a-1200w.jpg 1200w"
	if got := pickFromSrcset(srcset); got != "a-1200w.jpg" {
		t.Errorf("expected 1200w candidate, got %q", got)
	}
}

func TestPickFromSrcset_PicksHighestDensity(t *testing.T) {
	srcset := "a-1x.jpg 1x, a-2x.jpg 2x"
	if got := pickFromSrcset(srcset); got != "a-2x.jpg" {
		t.Errorf("expected 2x candidate, got %q", got)
	}
}

func TestExtractPageImages_SrcsetPicked(t *testing.T) {
	html := `<html><body><img srcset="/hero-480.jpg 480w, /hero-1200.jpg 1200w" alt="Hero"></body></html>`
	images := ExtractPageImages(html, "https://example.com", 10)
	if len(images) != 1 || images[0].URL != "https://example.com/hero-1200.jpg" {
		t.Fatalf("expected srcset max candidate, got %+v", images)
	}
}

func TestExtractPageImages_LazyLoadAttrs(t *testing.T) {
	html := `<html><body>
		<img src="data:image/gif;base64,R0lGOD" data-src="/lazy-real.jpg" alt="Lazy">
	</body></html>`
	images := ExtractPageImages(html, "https://example.com", 10)
	if len(images) != 1 || images[0].URL != "https://example.com/lazy-real.jpg" {
		t.Fatalf("expected data-src fallback, got %+v", images)
	}
}

func TestExtractPageImages_PictureSource(t *testing.T) {
	html := `<html><body>
		<picture><source srcset="/art-2000.webp 2000w"><img src="/art-fallback.jpg" alt="Art"></picture>
	</body></html>`
	images := ExtractPageImages(html, "https://example.com", 10)
	if len(images) != 1 || images[0].URL != "https://example.com/art-2000.webp" {
		t.Fatalf("expected picture source candidate, got %+v", images)
	}
}

func TestExtractPageImages_UnwrapsNextImageOptimizer(t *testing.T) {
	html := `<html><body><img src="/_next/image?url=%2Fhero.png&w=1200&q=75" alt="Hero"></body></html>`
	images := ExtractPageImages(html, "https://example.com", 10)
	if len(images) != 1 || images[0].URL != "https://example.com/hero.png" {
		t.Fatalf("expected unwrapped origin URL, got %+v", images)
	}
}

func TestExtractPageImages_BackgroundImage(t *testing.T) {
	html := `<html><body><div style="background-image: url('/banner.jpg')"></div></body></html>`
	images := ExtractPageImages(html, "https://example.com", 10)
	if len(images) != 1 || images[0].URL != "https://example.com/banner.jpg" || images[0].SourceType != "background" {
		t.Fatalf("expected background image, got %+v", images)
	}
}

func TestExtractPageImages_Ranking_BeatsAvatars(t *testing.T) {
	// Mirrors the real-world case observed on firecrawl.dev/blog: og:image,
	// one large hero, and several 48px author avatars. max=2 must return
	// og + hero, NOT the avatars.
	html := `<html><head><meta property="og:image" content="https://example.com/og.png"></head><body>
		<img src="/_next/image?url=%2Fimages%2Fblog%2Fhero.png&w=1200" alt="Hero story">
		<img src="/_next/image?url=%2Fblog%2Fauthors%2Falice.jpg&w=48" alt="Alice">
		<img src="/_next/image?url=%2Fblog%2Fauthors%2Fbob.jpg&w=48" alt="Bob">
		<img src="/_next/image?url=%2Fblog%2Fauthors%2Fcarol.jpg&w=48" alt="Carol">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com/blog", 2)
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d: %+v", len(images), images)
	}
	if images[0].SourceType != "og:image" {
		t.Errorf("rank 1 should be og:image, got %+v", images[0])
	}
	if images[1].SourceType != "hero" {
		t.Errorf("rank 2 should be hero, got %+v", images[1])
	}
	if strings.Contains(images[1].URL, "48") {
		t.Errorf("avatar should not outrank hero: %q", images[1].URL)
	}
}

func TestScoreImage_AvatarDetection(t *testing.T) {
	img := domain.ImageData{URL: "https://example.com/avatar.png", SourceType: sourceContent, Width: 48}
	if s := scoreImage(img); s != 10 {
		t.Errorf("expected avatar score 10, got %d", s)
	}
	hero := domain.ImageData{URL: "https://example.com/hero.png", SourceType: sourceHero, Width: 800}
	if s := scoreImage(hero); s != 75 {
		t.Errorf("expected hero score 75, got %d", s)
	}
}

func TestExtractPageImages_PriorityOrder(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://example.com/og.jpg">
		<meta name="twitter:image" content="https://example.com/twitter.jpg">
	</head><body>
		<img src="https://example.com/content.jpg" alt="Content">
	</body></html>`

	images := ExtractPageImages(html, "https://example.com", 10)

	if len(images) != 3 {
		t.Fatalf("Expected 3 images, got %d", len(images))
	}

	// OG should be first
	if images[0].SourceType != "og:image" {
		t.Errorf("First image should be og:image, got %q", images[0].SourceType)
	}

	// Twitter second
	if images[1].SourceType != "twitter:image" {
		t.Errorf("Second image should be twitter:image, got %q", images[1].SourceType)
	}

	// Content last
	if images[2].SourceType != "content" {
		t.Errorf("Third image should be content, got %q", images[2].SourceType)
	}
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		pageURL  string
		expected string
	}{
		{
			name:     "Absolute URL unchanged",
			rawURL:   "https://cdn.example.com/img.jpg",
			pageURL:  "https://example.com",
			expected: "https://cdn.example.com/img.jpg",
		},
		{
			name:     "Root-relative URL",
			rawURL:   "/images/photo.jpg",
			pageURL:  "https://example.com/blog/post",
			expected: "https://example.com/images/photo.jpg",
		},
		{
			name:     "Relative path",
			rawURL:   "photo.jpg",
			pageURL:  "https://example.com/blog/",
			expected: "https://example.com/blog/photo.jpg",
		},
		{
			name:     "Data URI returns empty",
			rawURL:   "data:image/png;base64,abc",
			pageURL:  "https://example.com",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveURL(tt.rawURL, tt.pageURL)
			if result != tt.expected {
				t.Errorf("resolveURL(%q, %q) = %q, want %q", tt.rawURL, tt.pageURL, result, tt.expected)
			}
		})
	}
}

func TestIsTrackingPixel(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com/tracking-pixel.gif", true},
		{"https://example.com/analytics/beacon.png", true},
		{"https://example.com/images/1x1.gif", true},
		{"https://example.com/spacer.gif", true},
		{"https://example.com/photo.jpg", false},
		{"https://cdn.example.com/hero-banner.webp", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := isTrackingPixel(tt.url)
			if result != tt.expected {
				t.Errorf("isTrackingPixel(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}
