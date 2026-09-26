package engines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultRegistry(t *testing.T) {
	reg, err := LoadRegistry("")
	if err != nil {
		t.Fatalf("bundled registry must load: %v", err)
	}
	specs := reg.Specs()
	if len(specs) < 15 {
		t.Errorf("expected the full roster, got %d engines", len(specs))
	}
	seen := map[string]bool{}
	for _, s := range specs {
		if seen[s.Name] {
			t.Errorf("duplicate engine %q", s.Name)
		}
		seen[s.Name] = true
		if err := validateSpec(s); err != nil {
			t.Errorf("spec %s invalid: %v", s.Name, err)
		}
	}
	// The 2026-09-26 in-house probe verdict: these are adopted; reuters and
	// stract were removed as unusable and must not come back silently.
	for _, want := range []string{"ddg", "bing", "wikipedia", "wikidata", "marginalia",
		"mwmbl", "brave", "bingnews", "wikinews", "hn", "github", "stackexchange",
		"mdn", "npm", "crates", "packagist", "mojeek"} {
		if !seen[want] {
			t.Errorf("roster missing adopted engine %q", want)
		}
	}
	for _, gone := range []string{"reuters", "stract", "yahoo", "yandex", "qwant", "ecosia"} {
		if seen[gone] {
			t.Errorf("engine %q was removed as unusable but is back in the roster", gone)
		}
	}
}

func TestRegistryOrderingAndDisabled(t *testing.T) {
	reg, err := LoadRegistry("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	specs := reg.Specs()
	for i := 1; i < len(specs); i++ {
		if specs[i-1].Weight < specs[i].Weight {
			t.Fatalf("roster not weight-sorted: %s(%d) before %s(%d)",
				specs[i-1].Name, specs[i-1].Weight, specs[i].Name, specs[i].Weight)
		}
	}
	enabled := reg.EnabledSpecs()
	for _, s := range enabled {
		if !s.IsEnabled() {
			t.Errorf("EnabledSpecs returned disabled engine %q", s.Name)
		}
	}
	// google + startpage ship disabled (plan R2 / M3) but stay documented.
	if g := reg.Spec("google"); g == nil || g.IsEnabled() {
		t.Error("google must exist and ship disabled by default")
	}
	if sp := reg.Spec("startpage"); sp == nil || sp.IsEnabled() {
		t.Error("startpage must exist and ship disabled by default")
	}
	if len(enabled) >= len(specs) {
		t.Error("expected at least one disabled engine in the roster")
	}
}

func TestRegistryOverrideWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.yaml")
	yaml := `
version: 1
engines:
  - name: only
    categories: [general]
    weight: 10
    request:
      url: "https://example.com/?q={{query}}"
    parse:
      type: rss
      fields:
        title: { path: title }
        url: { path: link }
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("override load: %v", err)
	}
	if got := len(reg.Specs()); got != 1 {
		t.Fatalf("override must replace wholesale, got %d engines", got)
	}
}

func TestRegistryValidation(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"unknown field", `
version: 1
engines:
  - name: x
    categories: [general]
    bogus: true
    request: { url: "https://e.com" }
    parse: { type: rss, fields: { title: {path: title}, url: {path: link} } }
`, "field bogus not found"},
		{"duplicate name", `
version: 1
engines:
  - name: dup
    categories: [general]
    request: { url: "https://e.com" }
    parse: { type: rss, fields: { title: {path: title}, url: {path: link} } }
  - name: dup
    categories: [general]
    request: { url: "https://e.com" }
    parse: { type: rss, fields: { title: {path: title}, url: {path: link} } }
`, "duplicate engine name"},
		{"bad category", `
version: 1
engines:
  - name: x
    categories: [video]
    request: { url: "https://e.com" }
    parse: { type: rss, fields: { title: {path: title}, url: {path: link} } }
`, "unknown category"},
		{"bad parse type", `
version: 1
engines:
  - name: x
    categories: [general]
    request: { url: "https://e.com" }
    parse: { type: regex, fields: { title: {path: title}, url: {path: link} } }
`, "unknown parse.type"},
		{"missing url field", `
version: 1
engines:
  - name: x
    categories: [general]
    request: { url: "https://e.com" }
    parse: { type: rss, fields: { title: {path: title} } }
`, "fields.url required"},
		{"unknown tls profile", `
version: 1
engines:
  - name: x
    categories: [general]
    profile: netscape
    request: { url: "https://e.com" }
    parse: { type: rss, fields: { title: {path: title}, url: {path: link} } }
`, "unknown TLS profile"},
		{"bad version", `
version: 2
engines:
  - name: x
    categories: [general]
    request: { url: "https://e.com" }
    parse: { type: rss, fields: { title: {path: title}, url: {path: link} } }
`, "unsupported registry version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "r.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadRegistry(path)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRegistryMissingFileFails(t *testing.T) {
	if _, err := LoadRegistry("/nonexistent/registry.yaml"); err == nil {
		t.Fatal("missing override file must fail loudly")
	}
}
