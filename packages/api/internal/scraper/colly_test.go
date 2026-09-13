package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// TestCollyScraper_ReadabilityMetadata verifies that readability main-content
// extraction populates metadata and strips boilerplate before markdown.
func TestCollyScraper_ReadabilityMetadata(t *testing.T) {
	page := `<html><head><title>Fixture Title</title>
<meta property="og:site_name" content="Fixture Site"></head>
<body><nav><a href="/x">Navigation junk link</a></nav>
<article><h1>Real Heading</h1>
<p>This is the actual article body with enough words for readability to
consider it the main content of the page.</p>
<p>Second paragraph keeps the candidate above the extraction threshold.</p>
</article><footer>Copyright footer noise that should not survive extraction.</footer></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	s := NewCollyScraper()
	result, err := s.Scrape(context.Background(), srv.URL, domain.ScrapeOptions{})
	if err != nil {
		t.Fatalf("scrape failed: %v", err)
	}

	if result.Metadata["title"] != "Fixture Title" {
		t.Errorf("expected title metadata, got %q", result.Metadata["title"])
	}
	if result.Metadata["site_name"] != "Fixture Site" {
		t.Errorf("expected site_name metadata, got %q", result.Metadata["site_name"])
	}
	if !strings.Contains(result.Markdown, "Real Heading") {
		t.Errorf("markdown missing article content: %s", result.Markdown)
	}
	if strings.Contains(result.Markdown, "footer noise") {
		t.Errorf("markdown should not contain footer boilerplate: %s", result.Markdown)
	}
}
