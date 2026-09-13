package extract

import (
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestApply_SingleAndMultiple(t *testing.T) {
	html := `<html><body>
		<h1>Product Title</h1>
		<p class="price">$49.99</p>
		<a class="link" href="/a">A</a><a class="link" href="/b">B</a>
	</body></html>`

	schema := map[string]domain.ExtractField{
		"title": {Selector: "h1"},
		"price": {Selector: ".price", Attr: "text"},
		"links": {Selector: "a.link", Attr: "href", Multiple: true},
	}

	out, err := Apply(html, schema)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if out["title"] != "Product Title" {
		t.Errorf("title = %v", out["title"])
	}
	if out["price"] != "$49.99" {
		t.Errorf("price = %v", out["price"])
	}
	links, ok := out["links"].([]string)
	if !ok || len(links) != 2 || links[0] != "/a" || links[1] != "/b" {
		t.Errorf("links = %v", out["links"])
	}
}

func TestApply_MissingSelectorOmitsField(t *testing.T) {
	out, err := Apply(`<html><body><h1>Hi</h1></body></html>`, map[string]domain.ExtractField{
		"missing": {Selector: ".nope"},
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if _, ok := out["missing"]; ok {
		t.Errorf("missing selector should omit field: %v", out)
	}
}

func TestApply_AttrHTML(t *testing.T) {
	html := `<html><body><div id="x"><b>bold</b></div></body></html>`
	out, err := Apply(html, map[string]domain.ExtractField{
		"html": {Selector: "#x", Attr: "html"},
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !strings.Contains(out["html"].(string), "<b>") {
		t.Errorf("html attr should include markup: %v", out["html"])
	}
}

func TestSummarize_PrefersExcerpt(t *testing.T) {
	got := Summarize("noisy markdown body", "The clean excerpt.", 5)
	if !strings.Contains(got, "clean excerpt") {
		t.Errorf("excerpt should win: %q", got)
	}
}

func TestSummarize_SentenceCount(t *testing.T) {
	md := "Alpha topic sentence with meaningful words here. " +
		"Beta continues the discussion with more content. " +
		"Gamma adds further detail to the story. " +
		"Delta wraps up the narrative completely. " +
		"Epsilon is a final closing remark."
	got := Summarize(md, "", 3)
	sentences := strings.Count(got, ".") // rough count incl. trailing
	if sentences > 4 {
		t.Errorf("summary too long: %q", got)
	}
	if got == "" {
		t.Error("summary should not be empty")
	}
}

func TestRedactPII(t *testing.T) {
	text := "Contact a.b+tag@example.com or +1 (555) 123-4567; card 4111 1111 1111 1111 today."
	got := RedactPII(text)
	for _, want := range []string{"[EMAIL]", "[PHONE]", "[CARD]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %q", want, got)
		}
	}
	if strings.Contains(got, "a.b+tag@example.com") || strings.Contains(got, "4111 1111") {
		t.Errorf("sensitive data not masked: %q", got)
	}
}

func TestRedactPII_LeavesProseIntact(t *testing.T) {
	got := RedactPII("The quick brown fox jumps over the lazy dog.")
	if got != "The quick brown fox jumps over the lazy dog." {
		t.Errorf("prose must be untouched: %q", got)
	}
}
