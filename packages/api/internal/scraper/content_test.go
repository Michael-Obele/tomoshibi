package scraper

import "testing"

func TestExtractMainContent_StripsBoilerplate(t *testing.T) {
	html := `<html><head><title>T</title></head><body><nav>nav junk</nav><article><h1>Hello</h1><p>Real content here with enough text to pass readability thresholds for extraction.</p></article><footer>footer junk</footer></body></html>`
	res, err := ExtractMainContent(html, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.ContentHTML == "" {
		t.Fatal("expected content")
	}
}

func TestExtractMainContent_FallsBackOnUnparseable(t *testing.T) {
	res, err := ExtractMainContent("<p>x</p>", "https://example.com")
	// must not error hard — fallback returns original
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected fallback result, got nil")
	}
}
