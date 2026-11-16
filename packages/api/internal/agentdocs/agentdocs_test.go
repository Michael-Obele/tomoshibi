package agentdocs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRouter(t *testing.T) (*gin.Engine, *Bundle) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	b, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := gin.New()
	b.Routes(r)
	return r, b
}

func get(r *gin.Engine, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestLLMSTxtFollowsTheSpec is the format contract. The llmstxt.org v2 order is
// H1, then a blockquote summary, then free-form guidance, then H2 sections of
// `[name](url): notes` lists. A parser relies on that order, so a regression here
// breaks every consumer silently.
func TestLLMSTxtFollowsTheSpec(t *testing.T) {
	r, b := newRouter(t)
	w := get(r, "/llms.txt", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}
	body := w.Body.String()
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")

	if !strings.HasPrefix(lines[0], "# ") {
		t.Fatalf("first line must be an H1, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[2], "> ") {
		t.Fatalf("line 3 must be the blockquote summary (blank line at 2), got %q", lines[2])
	}
	if !strings.Contains(body, "\n## ") {
		t.Fatalf("no H2 file-list section")
	}

	// Every published doc must be linked, or it is unreachable from the index.
	for _, d := range b.Manifest().Docs {
		if !strings.Contains(body, "]("+d.URL+")") {
			t.Errorf("%s is in the manifest but not linked from llms.txt", d.Slug)
		}
	}
	// The file has to stay small enough to sit in a context window; the whole
	// point is that the detail lives behind the links.
	if len(body) > 8000 {
		t.Errorf("llms.txt is %d bytes; keep the index small and put detail behind links", len(body))
	}
}

// TestEveryManifestDocIsServed: the manifest is a promise. A slug it lists that
// 404s is worse than no manifest at all.
func TestEveryManifestDocIsServed(t *testing.T) {
	r, b := newRouter(t)
	for _, d := range b.Manifest().Docs {
		w := get(r, d.URL, nil)
		if w.Code != http.StatusOK {
			t.Errorf("%s (%s) = %d", d.Slug, d.URL, w.Code)
			continue
		}
		if w.Body.Len() == 0 {
			t.Errorf("%s served an empty body", d.Slug)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
			t.Errorf("%s content-type = %q, want text/markdown", d.Slug, ct)
		}
	}
}

// TestVersionIsTheContract: same version means the same documentation. An agent
// caches by this, so it must change when the content does and be stable when it
// does not.
func TestVersionIsTheContract(t *testing.T) {
	r, b := newRouter(t)
	first := b.Manifest().Version
	if first == "" || first == "unknown" {
		t.Fatalf("version = %q", first)
	}
	second, err := Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if second.Manifest().Version != first {
		t.Fatalf("version is not stable across loads: %q then %q", first, second.Manifest().Version)
	}

	w := get(r, "/llms.txt", nil)
	if got := w.Header().Get("X-Docs-Version"); got != first {
		t.Fatalf("X-Docs-Version = %q, want %q", got, first)
	}
	if got := w.Header().Get("X-API-Version"); got != b.Manifest().APIVersion {
		t.Fatalf("X-API-Version = %q, want %q", got, b.Manifest().APIVersion)
	}
}

// TestRevalidationMakesCheapPollsFree: an agent should be able to ask "anything
// new?" without downloading the docs again.
func TestRevalidationMakesCheapPollsFree(t *testing.T) {
	r, b := newRouter(t)
	etag := b.Manifest().Version

	for _, path := range []string{"/llms.txt", "/llms-full.txt", "/docs", "/docs/api-reference.md"} {
		w := get(r, path, map[string]string{"If-None-Match": `"docs-` + etag + `"`})
		if w.Code != http.StatusNotModified {
			t.Errorf("%s with matching ETag = %d, want 304", path, w.Code)
		}
		if w.Body.Len() != 0 {
			t.Errorf("%s sent a body with a 304", path)
		}

		w = get(r, path, map[string]string{"If-None-Match": `"docs-stale"`})
		if w.Code != http.StatusOK {
			t.Errorf("%s with a stale ETag = %d, want 200", path, w.Code)
		}
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "must-revalidate") {
			t.Errorf("%s Cache-Control = %q, want must-revalidate (a stale cached copy is the bug)", path, cc)
		}
	}
}

// TestManifestIsMachineReadable: this is the route a program should call to
// decide whether to re-read anything.
func TestManifestIsMachineReadable(t *testing.T) {
	r, b := newRouter(t)
	w := get(r, "/docs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var m Manifest
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if m.Version != b.Manifest().Version || m.APIVersion == "" || m.DocCount != len(m.Docs) {
		t.Fatalf("manifest is inconsistent: %+v meta, %d docs", m, len(m.Docs))
	}
	for _, d := range m.Docs {
		if d.Slug == "" || d.Title == "" || d.Summary == "" || d.URL == "" || d.SHA256 == "" || d.Source == "" {
			t.Errorf("doc has an empty field: %+v", d)
		}
	}
}

// TestUnknownDocIsAnActionable404: an agent guessing a slug should be told where
// the list is, not just that it was wrong.
func TestUnknownDocIsAnActionable404(t *testing.T) {
	r, _ := newRouter(t)
	w := get(r, "/docs/does-not-exist.md", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/docs") {
		t.Fatalf("404 does not point at the manifest: %s", w.Body.String())
	}
}

// TestFullCorpusIsComplete: llms-full.txt is what an agent with a large window
// reads instead of the index, so it must contain every document.
func TestFullCorpusIsComplete(t *testing.T) {
	r, b := newRouter(t)
	w := get(r, "/llms-full.txt", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "# Tomoshibi") {
		t.Fatalf("llms-full.txt does not start with the H1")
	}
	for _, d := range b.Manifest().Docs {
		if !strings.Contains(body, "\n## "+d.Title+"\n") {
			t.Errorf("llms-full.txt is missing the %q section", d.Title)
		}
	}
}
