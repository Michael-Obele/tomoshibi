package scraper

import (
	"strings"
	"testing"
)

func TestShouldUseDynamic_NoscriptWithJS(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected bool
	}{
		{
			name:     "Enable javascript message",
			html:     `<html><body><noscript>You need to enable JavaScript to run this app.</noscript><div id="root"></div></body></html>`,
			expected: true,
		},
		{
			name:     "Need javascript message",
			html:     `<html><body><noscript>This app need JavaScript</noscript></body></html>`,
			expected: true,
		},
		{
			name:     "Requires javascript message",
			html:     `<html><body><noscript>This page requires javascript to work</noscript></body></html>`,
			expected: true,
		},
		{
			name:     "Noscript without JS requirement",
			html:     `<html><body><noscript>Please upgrade your browser</noscript><div>Content here</div></body></html>`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUseDynamic(tt.html)
			if result != tt.expected {
				t.Errorf("ShouldUseDynamic() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestShouldUseDynamic_SPARoots(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected bool
	}{
		{
			name:     "React root with small content",
			html:     `<html><body><div id="root"></div><script src="bundle.js"></script></body></html>`,
			expected: true,
		},
		{
			name:     "Next.js root with small content",
			html:     `<html><body><div id="__next"></div></body></html>`,
			expected: true,
		},
		{
			name:     "Angular root with small content",
			html:     `<html><body><app-root></app-root></body></html>`,
			expected: true,
		},
		{
			name:     "React root with large SSR content",
			html:     `<html><body><div id="root">` + strings.Repeat("Content ", 1000) + `</div></body></html>`,
			expected: false,
		},
		{
			name:     "data-reactroot with small content",
			html:     `<html><body><div data-reactroot>Tiny</div></body></html>`,
			expected: true,
		},
		{
			name:     "ng-version with small content",
			html:     `<html ng-version="15.0.0"><body><p>Angular</p></body></html>`,
			expected: true,
		},
		{
			name:     "__NEXT_DATA__ with small content",
			html:     `<html><body><script id="__NEXT_DATA__">{}</script></body></html>`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUseDynamic(tt.html)
			if result != tt.expected {
				t.Errorf("ShouldUseDynamic() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestShouldUseDynamic_ContentSize(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected bool
	}{
		{
			name:     "Tiny body with script tags",
			html:     `<html><body><script src="app.js"></script></body></html>`,
			expected: true,
		},
		{
			name:     "Large body with script tags",
			html:     `<html><body>` + strings.Repeat("<p>Large paragraph of content. ", 200) + `<script src="app.js"></script></body></html>`,
			expected: false,
		},
		{
			name:     "Tiny body without scripts",
			html:     `<html><body><p>Hello</p></body></html>`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUseDynamic(tt.html)
			if result != tt.expected {
				t.Errorf("ShouldUseDynamic() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestShouldUseDynamic_StaticSites(t *testing.T) {
	// Standard static HTML should not require dynamic scraping
	staticHTML := `
	<html>
	<head><title>My Blog</title></head>
	<body>
		<h1>Welcome to my blog</h1>
		<article>
			<h2>First Post</h2>
			<p>This is a standard blog post with plenty of content.
			   We have paragraphs and headings and lists.</p>
			<ul>
				<li>Item 1</li>
				<li>Item 2</li>
				<li>Item 3</li>
			</ul>
		</article>
	</body>
	</html>`

	if ShouldUseDynamic(staticHTML) {
		t.Error("Static HTML should not require dynamic scraping")
	}
}

func TestShouldUseDynamic_EmptyString(t *testing.T) {
	if ShouldUseDynamic("") {
		t.Error("Empty string should not require dynamic scraping")
	}
}
