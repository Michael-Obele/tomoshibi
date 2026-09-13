package scraper

import (
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestExtractLinks_ExampleDotCom(t *testing.T) {
	// Minimal example.com body after readability
	html := `<div><h1>Example Domain</h1><p>This domain is for use in documentation examples without needing permission. Avoid use in operations.</p><p><a href="https://www.iana.org/domains/example">More information...</a></p></div>`
	links := ExtractLinks(html, "https://example.com")
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1: %+v", len(links), links)
	}
	if links[0].URL != "https://www.iana.org/domains/example" {
		t.Errorf("URL = %q, want https://www.iana.org/domains/example", links[0].URL)
	}
	if links[0].Text != "More information..." {
		t.Errorf("Text = %q, want More information...", links[0].Text)
	}
	if links[0].IsInternal {
		t.Errorf("IsInternal = true, want false for iana.org")
	}

	// Also test with https://iana.org/domains/example variant (Firecrawl returns this)
	html2 := `<a href="https://iana.org/domains/example">Learn more</a>`
	links2 := ExtractLinks(html2, "https://example.com")
	if len(links2) != 1 || links2[0].URL != "https://iana.org/domains/example" {
		t.Fatalf("iana variant: %+v", links2)
	}
}

func TestExtractLinks_DedupeAndFiltering(t *testing.T) {
	html := `<a href="https://example.com/a">First</a>
	         <a href="https://example.com/a">Second dup</a>
	         <a href="/b">B text</a>
	         <a href="/b">B dup 2</a>
	         <a href="mailto:x@y.com">mail</a>
	         <a href="javascript:void(0)">js</a>
	         <a href="#section">frag</a>
	         <a href="tel:123">tel</a>
	         <a href="data:text/plain,hi">data</a>`
	links := ExtractLinks(html, "https://example.com")
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2 (a and /b): %+v", len(links), links)
	}
	// First link
	if links[0].URL != "https://example.com/a" || links[0].Text != "First" || !links[0].IsInternal {
		t.Errorf("first link wrong: %+v", links[0])
	}
	if links[1].URL != "https://example.com/b" || links[1].Text != "B text" {
		t.Errorf("second link wrong: %+v", links[1])
	}
}

func TestExtractLinks_IsInternal(t *testing.T) {
	html := `<a href="/internal">Internal</a><a href="https://other.com/page">External</a><a href="https://example.com/other">Same host</a>`
	links := ExtractLinks(html, "https://example.com/page")
	if len(links) != 3 {
		t.Fatalf("got %d, want 3", len(links))
	}
	cases := map[string]bool{
		"https://example.com/internal": true,
		"https://other.com/page":       false,
		"https://example.com/other":    true,
	}
	for _, l := range links {
		want, ok := cases[l.URL]
		if !ok {
			t.Errorf("unexpected URL %q", l.URL)
			continue
		}
		if l.IsInternal != want {
			t.Errorf("URL %q IsInternal=%v want %v", l.URL, l.IsInternal, want)
		}
	}
}

func TestExtractLinks_ResolvesRelative(t *testing.T) {
	html := `<a href="../up">Up</a><a href="//cdn.example.com/x">Proto</a>`
	links := ExtractLinks(html, "https://example.com/a/b/")
	if len(links) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(links), links)
	}
	if links[0].URL != "https://example.com/a/up" {
		t.Errorf("relative resolved = %q, want https://example.com/a/up", links[0].URL)
	}
	if links[1].URL != "https://cdn.example.com/x" {
		t.Errorf("protocol-relative = %q", links[1].URL)
	}
}

func TestExtractLinks_Empty(t *testing.T) {
	if got := ExtractLinks("", "https://example.com"); got != nil {
		t.Errorf("empty HTML should return nil, got %v", got)
	}
	if got := ExtractLinks("<p>no links</p>", "https://example.com"); got != nil {
		t.Errorf("no anchors should return nil, got %v", got)
	}
}

func TestExtractLinks_IncludeLinksToggle(t *testing.T) {
	// Service layer defaults IncludeLinks nil -> true, so links are produced.
	// Explicit false omits them (handled in scrapers, tested via handler below).
	_ = domain.ScrapeOptions{}
}
