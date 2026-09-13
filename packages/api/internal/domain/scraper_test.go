package domain

import (
	"encoding/json"
	"testing"
)

func TestScrapeResult_JSONMarshal(t *testing.T) {
	result := ScrapeResult{
		URL:      "https://example.com",
		Markdown: "# Example",
		HTML:     "<html><body><h1>Example</h1></body></html>",
		Metadata: map[string]string{
			"engine":     "colly",
			"scraped_at": "2026-01-01T00:00:00Z",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal ScrapeResult: %v", err)
	}

	var decoded ScrapeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ScrapeResult: %v", err)
	}

	if decoded.URL != result.URL {
		t.Errorf("URL mismatch: got %q, want %q", decoded.URL, result.URL)
	}
	if decoded.Markdown != result.Markdown {
		t.Errorf("Markdown mismatch: got %q, want %q", decoded.Markdown, result.Markdown)
	}
	if decoded.HTML != result.HTML {
		t.Errorf("HTML mismatch: got %q, want %q", decoded.HTML, result.HTML)
	}
	if decoded.Metadata["engine"] != "colly" {
		t.Errorf("Metadata engine mismatch: got %q, want %q", decoded.Metadata["engine"], "colly")
	}
}

func TestScrapeResult_JSONOmitEmpty(t *testing.T) {
	result := ScrapeResult{
		URL:      "https://example.com",
		Markdown: "# Example",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Failed to unmarshal raw: %v", err)
	}

	// HTML and Metadata should be omitted (omitempty tags)
	if _, exists := raw["html"]; exists {
		t.Error("HTML should be omitted when empty")
	}
	if _, exists := raw["metadata"]; exists {
		t.Error("Metadata should be omitted when nil")
	}

	// URL and Markdown should always be present
	if _, exists := raw["url"]; !exists {
		t.Error("URL should always be present")
	}
	if _, exists := raw["markdown"]; !exists {
		t.Error("Markdown should always be present")
	}
}

func TestScrapeResult_ZeroValue(t *testing.T) {
	var result ScrapeResult

	if result.URL != "" {
		t.Error("Zero-value URL should be empty")
	}
	if result.Markdown != "" {
		t.Error("Zero-value Markdown should be empty")
	}
	if result.HTML != "" {
		t.Error("Zero-value HTML should be empty")
	}
	if result.Metadata != nil {
		t.Error("Zero-value Metadata should be nil")
	}
}

func TestScrapeResult_LargeContent(t *testing.T) {
	// Simulate a large HTML page
	largeHTML := ""
	for i := 0; i < 1000; i++ {
		largeHTML += "<div>Content block</div>"
	}

	result := ScrapeResult{
		URL:      "https://example.com/large-page",
		Markdown: "# Large page\nContent block",
		HTML:     largeHTML,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal large ScrapeResult: %v", err)
	}

	var decoded ScrapeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal large ScrapeResult: %v", err)
	}

	if len(decoded.HTML) != len(result.HTML) {
		t.Errorf("Large HTML content lost during marshal/unmarshal: got len %d, want len %d",
			len(decoded.HTML), len(result.HTML))
	}
}
