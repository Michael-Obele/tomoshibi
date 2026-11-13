package recipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"go.yaml.in/yaml/v3"
)

const validYAML = `
id: demo
version: 1
match: { domains: ["example.com", "*.example.org"] }
access:
  lane: borrow
  io: turbo
  pace: { min_interval_ms: 500 }
request:
  actions:
    - { type: click, selector: "#expand" }
    - { type: type, selector: "#q", text: "hello" }
  wait_selector: "#content"
extract:
  schema:
    title: { selector: h1, attr: text }
expect:
  status: 200
  absent: ["Just a moment..."]
fixtures:
  - { url: "https://example.com/", file: "fixtures/home.html", expect: { title: "Hello" } }
`

func TestParseAndValidate(t *testing.T) {
	r, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.IO() != IOTurbo {
		t.Fatalf("IO = %q", r.IO())
	}
	if r.Ref() != "demo@1" {
		t.Fatalf("Ref = %q", r.Ref())
	}

	bad := []struct{ name, yaml string }{
		{"no id", "version: 1\nmatch: { domains: [\"a.com\"] }\naccess: { lane: borrow }\nfixtures: [{url: u, file: f}]"},
		{"bad lane", "id: x\nversion: 1\nmatch: { domains: [\"a.com\"] }\naccess: { lane: sneaky }\nfixtures: [{url: u, file: f}]"},
		{"bad action", "id: x\nversion: 1\nmatch: { domains: [\"a.com\"] }\naccess: { lane: borrow }\nrequest: { actions: [{ type: exfiltrate }] }\nfixtures: [{url: u, file: f}]"},
		{"no domains", "id: x\nversion: 1\naccess: { lane: borrow }\nfixtures: [{url: u, file: f}]"},
		{"no fixtures", "id: x\nversion: 1\nmatch: { domains: [\"a.com\"] }\naccess: { lane: borrow }"},
		{"fixture without file", "id: x\nversion: 1\nmatch: { domains: [\"a.com\"] }\naccess: { lane: borrow }\nfixtures: [{url: u}]"},
	}
	for _, c := range bad {
		if _, err := Parse([]byte(c.yaml)); err == nil {
			t.Errorf("%s: expected validation error", c.name)
		}
	}
}

func TestHostMatches(t *testing.T) {
	cases := []struct {
		pattern, host string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "www.example.com", false},
		{"*.example.com", "www.example.com", true},
		{"*.example.com", "example.com", false},
		{"EXAMPLE.com", "example.com", true},
	}
	for _, c := range cases {
		if got := hostMatches(c.pattern, c.host); got != c.want {
			t.Errorf("hostMatches(%q, %q) = %v, want %v", c.pattern, c.host, got, c.want)
		}
	}
}

// TestMatchSpecificity: an exact host must beat a wildcard covering the same
// host, and the winner must not depend on which recipe was loaded first.
func TestMatchSpecificity(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		// The wildcard sorts first in id order, so a length-only compare would
		// pick it.
		"sites/a-wild.yaml": `id: a-wild
version: 1
match: { domains: ["*.example.com"] }
access: { lane: borrow, io: scripting }
fixtures: [{ url: "http://x.example.com/", file: "f.html" }]
`,
		"sites/z-exact.yaml": `id: z-exact
version: 1
match: { domains: ["m.example.com"] }
access: { lane: borrow, io: scripting }
fixtures: [{ url: "http://m.example.com/", file: "f.html" }]
`,
	}
	writePack(t, dir, files)
	writePack(t, dir, map[string]string{"fixtures/f.html": "<html></html>"})
	withManifest(t, dir, manifestFor(t, dir, "spec", []string{"sites/a-wild.yaml", "sites/z-exact.yaml", "fixtures/f.html"}))
	stateDir := t.TempDir()
	if _, _, err := Install(context.Background(), dir, stateDir); err != nil {
		t.Fatalf("Install: %v", err)
	}
	reg := LoadRegistry(stateDir)
	if r := reg.Match("m.example.com"); r == nil || r.ID != "z-exact" {
		t.Fatalf("Match(m.example.com) = %+v, want the exact-host recipe", r)
	}
	if r := reg.Match("other.example.com"); r == nil || r.ID != "a-wild" {
		t.Fatalf("Match(other.example.com) = %+v, want the wildcard recipe", r)
	}
}

// writePack builds a minimal on-disk pack and returns its directory.
func writePack(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func withManifest(t *testing.T, dir string, manifest Manifest) {
	t.Helper()
	raw, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ManifestName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func manifestFor(t *testing.T, dir string, name string, paths []string) Manifest {
	t.Helper()
	m := Manifest{Name: name, Version: 1}
	for _, p := range paths {
		sum, err := fileSHA256(filepath.Join(dir, p))
		if err != nil {
			t.Fatal(err)
		}
		m.Files = append(m.Files, ManifestFile{Path: p, SHA256: sum})
	}
	return m
}

func TestManifestVerifyAndTamper(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir, map[string]string{
		"sites/demo.yaml":    validYAML,
		"fixtures/home.html": "<html><h1>Hello</h1></html>",
	})
	withManifest(t, dir, manifestFor(t, dir, "demo-pack", []string{"sites/demo.yaml", "fixtures/home.html"}))

	if _, err := VerifyManifest(dir); err != nil {
		t.Fatalf("clean pack must verify: %v", err)
	}

	// A tampered file is caught.
	if err := os.WriteFile(filepath.Join(dir, "fixtures/home.html"), []byte("<html><h1>Evil</h1></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyManifest(dir); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("tampered file must fail verification, got %v", err)
	}

	// An unlisted file is caught too.
	if err := os.WriteFile(filepath.Join(dir, "fixtures/extra.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyManifest(dir); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("unlisted file must fail verification, got %v", err)
	}
}

func TestInstallAndFixtureVerify(t *testing.T) {
	stateDir := t.TempDir()
	src := t.TempDir()
	writePack(t, src, map[string]string{
		"sites/demo.yaml":    validYAML,
		"fixtures/home.html": "<html><body><h1>Hello</h1></body></html>",
	})
	withManifest(t, src, manifestFor(t, src, "demo-pack", []string{"sites/demo.yaml", "fixtures/home.html"}))

	m, dest, err := Install(context.Background(), src, stateDir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if m.Name != "demo-pack" || !strings.HasPrefix(filepath.Base(dest), "demo-pack@") {
		t.Fatalf("installed at %s as %+v", dest, m)
	}

	// Installing the same content again is a no-op, not an error.
	if _, dest2, err := Install(context.Background(), src, stateDir); err != nil || dest2 != dest {
		t.Fatalf("second install: %v %s", err, dest2)
	}

	reg := LoadRegistry(stateDir)
	if r := reg.Match("example.com"); r == nil || r.ID != "demo" {
		t.Fatalf("Match(example.com) = %+v", r)
	}
	if r := reg.Match("sub.example.org"); r == nil || r.ID != "demo" {
		t.Fatalf("Match(sub.example.org) = %+v", r)
	}
	if r := reg.Match("elsewhere.net"); r != nil {
		t.Fatalf("Match(elsewhere.net) = %+v, want nil", r)
	}

	// Fixture replay passes on the good page.
	loaded, ok := reg.ByID("demo")
	if !ok {
		t.Fatal("demo not found by id")
	}
	results, err := VerifyFixtures(loaded)
	if err != nil {
		t.Fatalf("VerifyFixtures: %v", err)
	}
	if len(results) != 1 || !results[0].Pass {
		t.Fatalf("fixture should pass: %+v", results)
	}

	// A fixture that contains a block marker fails: the saved page is a
	// challenge, not content.
	loaded.Fixtures[0].File = "fixtures/challenge.html"
	writePack(t, dest, map[string]string{"fixtures/challenge.html": "<html><body>Just a moment...</body></html>"})
	results, err = VerifyFixtures(loaded)
	if err != nil {
		t.Fatalf("VerifyFixtures: %v", err)
	}
	if results[0].Pass {
		t.Fatal("challenge fixture must fail the absent check")
	}

	// A changed page that no longer yields the expected field fails too.
	loaded.Fixtures[0].File = "fixtures/moved.html"
	writePack(t, dest, map[string]string{"fixtures/moved.html": "<html><body><h1>Renamed</h1></body></html>"})
	results, _ = VerifyFixtures(loaded)
	if results[0].Pass || !strings.Contains(results[0].Detail, "title") {
		t.Fatalf("drifted fixture must fail naming the field: %+v", results[0])
	}
}

func TestRegistryReloadsOnChange(t *testing.T) {
	stateDir := t.TempDir()
	reg := LoadRegistry(stateDir)
	if r := reg.Match("example.com"); r != nil {
		t.Fatalf("empty registry matched %+v", r)
	}

	src := t.TempDir()
	writePack(t, src, map[string]string{
		"sites/demo.yaml":    validYAML,
		"fixtures/home.html": "<html><body><h1>Hello</h1></body></html>",
	})
	withManifest(t, src, manifestFor(t, src, "demo-pack", []string{"sites/demo.yaml", "fixtures/home.html"}))
	if _, _, err := Install(context.Background(), src, stateDir); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// No restart: the registry notices the new pack.
	if r := reg.Match("example.com"); r == nil || r.ID != "demo" {
		t.Fatalf("registry did not reload; Match = %+v", r)
	}
}

func TestSplitPackDir(t *testing.T) {
	name, sha := splitPackDir("demo-pack@abc123def456")
	if name != "demo-pack" || sha != "abc123def456" {
		t.Fatalf("got %q %q", name, sha)
	}
	name, sha = splitPackDir("plain")
	if name != "plain" || sha != "" {
		t.Fatalf("got %q %q", name, sha)
	}
}

var _ = domain.ExtractField{} // keep the domain import meaningful for reviewers
