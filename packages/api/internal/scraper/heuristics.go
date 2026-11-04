package scraper

import (
	"strings"
)

// shellTextThreshold is how much visible text a document may contain before
// it stops being an SPA shell. Framework markers are a weak signal: a compact
// server-rendered Next.js page carries __NEXT_DATA__ and is still complete.
// Judging shell-ness by raw HTML size spawned a browser for exactly those
// pages, which is the expensive direction to be wrong in.
const shellTextThreshold = 500

// ShouldUseDynamic analyzes HTML content to decide if dynamic rendering
// (headless browser) is needed.
func ShouldUseDynamic(htmlBody string) bool {
	lowerBody := strings.ToLower(htmlBody)

	// 1. Check for <noscript> instructions. This is the strongest signal: the
	// page itself says it needs JavaScript, regardless of how much markup it
	// shipped.
	if strings.Contains(lowerBody, "<noscript>") {
		if strings.Contains(lowerBody, "enable javascript") ||
			strings.Contains(lowerBody, "need javascript") ||
			strings.Contains(lowerBody, "requires javascript") {
			return true
		}
	}

	// 2. SPA roots only matter when the page is actually a shell. Measure the
	// text a reader would see, not the bytes in the document.
	if visibleTextLen(lowerBody) > shellTextThreshold {
		return false
	}

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
			return true
		}
	}

	// 3. A tiny document with scripts and no readable text is a shell even
	// without a recognised framework marker.
	if len(htmlBody) < 2000 && strings.Contains(lowerBody, "<script") {
		return true
	}

	return false
}

// visibleTextLen estimates how many non-whitespace characters a reader would
// see, skipping tags, script bodies, and style bodies. It is deliberately a
// scanner rather than a DOM parse: this runs on every static scrape and only
// needs to separate a shell from a rendered page.
func visibleTextLen(s string) int {
	const (
		scriptOpen  = "<script"
		scriptClose = "</script>"
		styleOpen   = "<style"
		styleClose  = "</style>"
	)

	count := 0
	inTag := false
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], scriptOpen) {
			end := strings.Index(s[i:], scriptClose)
			if end < 0 {
				return count
			}
			i += end + len(scriptClose)
			continue
		}
		if strings.HasPrefix(s[i:], styleOpen) {
			end := strings.Index(s[i:], styleClose)
			if end < 0 {
				return count
			}
			i += end + len(styleClose)
			continue
		}

		switch c := s[i]; {
		case c == '<':
			inTag = true
		case c == '>':
			inTag = false
		case !inTag && c != ' ' && c != '\n' && c != '\r' && c != '\t':
			count++
		}
		i++
	}
	return count
}
