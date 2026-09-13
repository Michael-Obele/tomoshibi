package scraper

import (
	"strings"
)

// ShouldUseDynamic analyzes HTML content to decide if dynamic rendering (headless browser) is needed.
// It returns true if the content appears to be an SPA shell or explicitly requires JavaScript.
func ShouldUseDynamic(htmlBody string) bool {
	lowerBody := strings.ToLower(htmlBody)

	// 1. Check for <noscript> instructions
	// Many SPAs put "You need to enable JavaScript" inside noscript tags.
	if strings.Contains(lowerBody, "<noscript>") {
		// We could be more specific, but presence of noscript is a strong hint
		// that the site expects JS-disabled clients (like us) to miss out.
		// Detailed checks:
		if strings.Contains(lowerBody, "enable javascript") ||
			strings.Contains(lowerBody, "need javascript") ||
			strings.Contains(lowerBody, "requires javascript") {
			return true
		}
	}

	// 2. Check for SPA Root Elements
	// React: id="root", id="app", data-reactroot
	// Next.js: id="__next", __NEXT_DATA__
	// Vue: id="app", data-v-
	spaRoots := []string{
		`id="root"`,
		`id="app"`,
		`id="__next"`,
		`data-reactroot`,
		`__NEXT_DATA__`,
		`ng-version`, // Angular
		`<app-root>`, // Angular
	}

	for _, marker := range spaRoots {
		if strings.Contains(htmlBody, marker) {
			// Just having a root isn't enough (SSR might fill it),
			// but combined with small content size, it's a strong indicator.
			// Let's refine this: if we see these AND the body is relatively small, assume shell.
			if len(htmlBody) < 5000 {
				return true
			}
		}
	}

	// 3. Simple Content Size Heuristic
	// If the body is tiny (< 2KB) but has script tags, it's likely a shell.
	if len(htmlBody) < 2000 && strings.Contains(lowerBody, "<script") {
		return true
	}

	return false
}
