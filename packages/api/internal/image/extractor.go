package image

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/PuerkitoBio/goquery"
)

// Source type labels recorded on extracted images.
const (
	sourceOG         = "og:image"
	sourceTwitter    = "twitter:image"
	sourceHero       = "hero"
	sourceContent    = "content"
	sourceBackground = "background"
)

// maxBackgroundImages caps how many CSS background-image candidates are
// collected to avoid noise from decorative elements.
const maxBackgroundImages = 5

// ExtractPageImages parses HTML and extracts image metadata.
// It prioritizes OG images, Twitter card images, then content images
// (including srcset candidates, lazy-loaded images, and <picture> sources),
// ranked by quality so maxImages returns the most valuable picks.
func ExtractPageImages(htmlBody string, pageURL string, maxImages int) []domain.ImageData {
	if maxImages <= 0 {
		maxImages = 10
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return nil
	}

	images := []domain.ImageData{}
	seen := make(map[string]bool)

	addImage := func(rawURL, alt, title, sourceType string) {
		if rawURL == "" {
			return
		}
		rawURL = unwrapOptimizer(rawURL)
		absURL := resolveURL(rawURL, pageURL)
		if absURL == "" || seen[absURL] || isTrackingPixel(absURL) {
			return
		}
		seen[absURL] = true
		images = append(images, domain.ImageData{
			URL:        absURL,
			Alt:        alt,
			Title:      title,
			SourceType: sourceType,
		})
	}

	// 1. OG Image (highest priority for AI consumption)
	if ogImage, exists := doc.Find(`meta[property="og:image"]`).Attr("content"); exists {
		addImage(ogImage, "", "", sourceOG)
	}

	// 2. Twitter card image
	doc.Find(`meta[property="twitter:image"], meta[name="twitter:image"]`).Each(func(i int, s *goquery.Selection) {
		if content, exists := s.Attr("content"); exists {
			addImage(content, "", "", sourceTwitter)
		}
	})

	// 3. Content images (with srcset/lazy/picture support)
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		if len(images) >= maxImages*3 { // generous cap; ranking trims later
			return
		}

		alt, _ := s.Attr("alt")
		title, _ := s.Attr("title")

		src := pickImageSource(s)
		if src == "" {
			return
		}
		if strings.HasPrefix(src, "data:") {
			return
		}

		img := domain.ImageData{URL: src, Alt: alt, Title: title, SourceType: sourceContent}
		if w, _ := s.Attr("width"); w != "" {
			if v, err := strconv.Atoi(w); err == nil {
				img.Width = v
			}
		}
		if h, _ := s.Attr("height"); h != "" {
			if v, err := strconv.Atoi(h); err == nil {
				img.Height = v
			}
		}
		// Image optimizers embed the rendered width in the query string
		// (e.g. Next.js _next/image?url=...&w=48) — use it for scoring.
		if w, ok := optimizerWidthHint(src); ok && img.Width == 0 {
			img.Width = w
		}
		// Hero: content image with explicit dimensions of at least 300px.
		if img.Width >= 300 {
			img.SourceType = sourceHero
		}

		absURL := unwrapOptimizer(img.URL)
		absURL = resolveURL(absURL, pageURL)
		if absURL == "" || seen[absURL] || isTrackingPixel(absURL) {
			return
		}
		seen[absURL] = true
		img.URL = absURL
		images = append(images, img)
	})

	// 4. CSS background images (decorative/hero banners)
	var bgCount int
	doc.Find("[style*='background-image']").Each(func(i int, s *goquery.Selection) {
		if bgCount >= maxBackgroundImages {
			return
		}
		style, _ := s.Attr("style")
		if raw := extractBackgroundURL(style); raw != "" {
			addImage(raw, "", "", sourceBackground)
			bgCount++
		}
	})

	// 5. Rank and truncate to maxImages.
	rankImages(images)
	if len(images) > maxImages {
		images = images[:maxImages]
	}
	return images
}

// pickImageSource returns the best image URL for an <img> element:
// srcset (own or <picture><source>), then lazy-load attrs, then src
// (unless src is a placeholder data: URI).
func pickImageSource(s *goquery.Selection) string {
	// Own srcset first.
	if srcset, exists := s.Attr("srcset"); exists {
		if u := pickFromSrcset(srcset); u != "" {
			return u
		}
	}
	// <picture><source srcset> — first source wins per HTML spec.
	if parent := s.Parent(); parent != nil && parent.Is("picture") {
		if srcset, exists := parent.Find("source[srcset]").First().Attr("srcset"); exists {
			if u := pickFromSrcset(srcset); u != "" {
				return u
			}
		}
	}
	// Lazy-load attributes.
	for _, attr := range []string{"data-srcset", "data-src", "data-lazy-src", "data-original"} {
		if u, exists := s.Attr(attr); exists && u != "" {
			if strings.HasPrefix(attr, "data-srcset") {
				if picked := pickFromSrcset(u); picked != "" {
					return picked
				}
				continue
			}
			return u
		}
	}
	// Plain src, unless it's an inline placeholder.
	if src, exists := s.Attr("src"); exists && src != "" && !strings.HasPrefix(src, "data:") {
		return src
	}
	return ""
}

// pickFromSrcset selects the highest-resolution candidate from a srcset
// attribute (largest width descriptor, falling back to highest density).
func pickFromSrcset(srcset string) string {
	var best string
	bestW := -1
	bestD := -1.0
	for _, part := range strings.Split(srcset, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		candURL := fields[0]
		var descriptor string
		if len(fields) > 1 {
			descriptor = fields[1]
		}
		switch {
		case strings.HasSuffix(descriptor, "w"):
			if w, err := strconv.Atoi(strings.TrimSuffix(descriptor, "w")); err == nil && w > bestW {
				bestW, best = w, candURL
			}
		case strings.HasSuffix(descriptor, "x"):
			if d, err := strconv.ParseFloat(strings.TrimSuffix(descriptor, "x"), 64); err == nil && d > bestD {
				bestD, best = d, candURL
			}
		case bestW < 0 && bestD < 0:
			best = candURL
		}
	}
	return best
}

// unwrapOptimizer converts image-optimizer URLs (e.g. Next.js _next/image)
// back to the origin image URL so clients fetch the real asset.
func unwrapOptimizer(rawURL string) string {
	if !strings.Contains(rawURL, "_next/image") {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if target := u.Query().Get("url"); target != "" {
		return target
	}
	return rawURL
}

// optimizerWidthHint extracts the rendered width from image-optimizer URLs
// (Next.js _next/image uses a w= query param).
func optimizerWidthHint(rawURL string) (int, bool) {
	if !strings.Contains(rawURL, "_next/image") {
		return 0, false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, false
	}
	w := u.Query().Get("w")
	if w == "" {
		return 0, false
	}
	n, err := strconv.Atoi(w)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// extractBackgroundURL pulls the first url(...) out of a style attribute.
func extractBackgroundURL(style string) string {
	idx := strings.Index(style, "url(")
	if idx == -1 {
		return ""
	}
	rest := style[idx+4:]
	end := strings.IndexByte(rest, ')')
	if end == -1 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(rest[:end]), `"'`)
}

// rankImages sorts images by quality score, descending, stable.
func rankImages(images []domain.ImageData) {
	// Simple stable insertion sort keeps ordering for equal scores.
	for i := 1; i < len(images); i++ {
		for j := i; j > 0 && scoreImage(images[j]) > scoreImage(images[j-1]); j-- {
			images[j], images[j-1] = images[j-1], images[j]
		}
	}
}

// scoreImage returns a quality score for ranking image picks.
func scoreImage(img domain.ImageData) int {
	switch img.SourceType {
	case sourceOG:
		return 100
	case sourceTwitter:
		return 90
	case sourceHero:
		return 75
	case sourceContent:
		lower := strings.ToLower(img.URL)
		if img.Width > 0 && img.Width <= 128 {
			return 10
		}
		if strings.Contains(lower, "avatar") || strings.Contains(lower, "icon") ||
			strings.Contains(lower, "logo") || strings.Contains(lower, "sprite") {
			return 10
		}
		return 40
	case sourceBackground:
		return 30
	}
	return 30
}

func resolveURL(rawURL, pageURL string) string {
	if strings.HasPrefix(rawURL, "data:") {
		return ""
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
		"1x1", "spacer", "blank",
	}
	lower := strings.ToLower(imgURL)
	for _, t := range trackers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}
