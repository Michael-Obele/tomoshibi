package compat

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/search"
)

type stubReporter struct {
	results []search.Result
	report  search.Report
	err     error
	gotOpts search.SearchOptions
}

func (s *stubReporter) SearchWithReport(_ context.Context, opts search.SearchOptions) ([]search.Result, int, search.Report, error) {
	s.gotOpts = opts
	if s.err != nil {
		return nil, 0, s.report, s.err
	}
	return s.results, len(s.results), s.report, nil
}

func goldenSet(t *testing.T, file string) (topKeys map[string]bool, resultKeys map[string]bool) {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatalf("read golden %s: %v (fixtures come from plan/tomoshi-search/assets)", file, err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse golden %s: %v", file, err)
	}
	topKeys = map[string]bool{}
	for k := range doc {
		topKeys[k] = true
	}
	var results []map[string]json.RawMessage
	if err := json.Unmarshal(doc["results"], &results); err != nil {
		t.Fatalf("golden results: %v", err)
	}
	resultKeys = map[string]bool{}
	for _, r := range results {
		for k := range r {
			resultKeys[k] = true
		}
	}
	return topKeys, resultKeys
}

func serve(t *testing.T, svc Reporter, target string) (int, map[string]json.RawMessage) {
	t.Helper()
	h := NewHandler(svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", target, nil)
	h.ServeHTTP(rec, req)
	var body map[string]json.RawMessage
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("response not JSON: %v\n%s", err, rec.Body.String())
		}
	}
	return rec.Code, body
}

// TestContractAgainstGoldens pins the SearXNG JSON shape: identical
// top-level key set, result elements that are a key-superset of every
// golden element, and unresponsive_engines as [name, reason] tuples.
func TestContractAgainstGoldens(t *testing.T) {
	pub := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	svc := &stubReporter{
		results: []search.Result{
			{Title: "One", URL: "https://example.com/one", Description: "first", Engine: "ddg", Relevance: 0.9},
			{Title: "Two", URL: "https://example.com/two?q=1", Description: "second", Engine: "bing", Relevance: 0.7, PublishedAt: &pub},
		},
		report: search.Report{Outcomes: []search.EngineOutcome{
			{Engine: "mojeek", Err: context.DeadlineExceeded},
		}},
	}

	code, body := serve(t, svc, "/search?q=hello&format=json")
	if code != 200 {
		t.Fatalf("status = %d", code)
	}

	wantTop, wantResult := goldenSet(t, "searxng-general-golden.json")
	gotTop := map[string]bool{}
	for k := range body {
		gotTop[k] = true
	}
	for k := range wantTop {
		if !gotTop[k] {
			t.Errorf("missing top-level key %q (golden contract)", k)
		}
	}
	for k := range gotTop {
		if !wantTop[k] {
			t.Errorf("unexpected top-level key %q (golden contract)", k)
		}
	}
	// Same for the it-category fixture (its element keys are the core set).
	_, wantResultIt := goldenSet(t, "searxng-it-golden.json")
	for k := range wantResultIt {
		wantResult[k] = true
	}

	var results []map[string]json.RawMessage
	if err := json.Unmarshal(body["results"], &results); err != nil {
		t.Fatalf("results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for k := range wantResult {
		if _, ok := results[0][k]; !ok {
			t.Errorf("result element missing golden key %q", k)
		}
	}

	var unresp [][2]string
	if err := json.Unmarshal(body["unresponsive_engines"], &unresp); err != nil {
		t.Fatalf("unresponsive_engines tuple shape: %v", err)
	}
	if len(unresp) != 1 || unresp[0][0] != "mojeek" {
		t.Fatalf("unresponsive = %+v", unresp)
	}

	// Sanity on a few semantic fields.
	var full compatResponse
	raw, _ := json.Marshal(body)
	if err := json.Unmarshal(raw, &full); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if full.Results[0].Engine != "ddg" || full.Results[0].Engines[0] != "ddg" {
		t.Errorf("engine attribution: %+v", full.Results[0])
	}
	if full.Results[0].Template != "default.html" || full.Results[0].Category != "general" {
		t.Errorf("template/category: %+v", full.Results[0])
	}
	if got := full.Results[0].ParsedURL; got[0] != "https" || got[1] != "example.com" {
		t.Errorf("parsed_url = %v", got)
	}
	if full.Results[1].PublishedDate == nil || full.Results[1].Pubdate == "" {
		t.Error("publishedDate/pubdate not populated from PublishedAt")
	}
	for _, k := range []string{"answers", "corrections", "infoboxes", "suggestions"} {
		if string(body[k]) != "[]" {
			t.Errorf("%s = %s, want []", k, body[k])
		}
	}
}

func TestParamMapping(t *testing.T) {
	svc := &stubReporter{results: []search.Result{
		{Title: "A", URL: "https://a.dev/1", Engine: "keepme"},
		{Title: "B", URL: "https://b.dev/2", Engine: "other"},
	}}

	tests := []struct {
		target string
		check  func(t *testing.T, o search.SearchOptions)
	}{
		{"/search?q=x&categories=it", func(t *testing.T, o search.SearchOptions) {
			if o.Category != "code" {
				t.Errorf("categories=it → Category %q, want code", o.Category)
			}
		}},
		{"/search?q=x&categories=news", func(t *testing.T, o search.SearchOptions) {
			if o.Category != "news" {
				t.Errorf("Category %q", o.Category)
			}
		}},
		{"/search?q=x&pageno=3", func(t *testing.T, o search.SearchOptions) {
			if o.Offset != 40 {
				t.Errorf("pageno=3 → offset %d, want 40", o.Offset)
			}
		}},
		{"/search?q=x&time_range=week", func(t *testing.T, o search.SearchOptions) {
			if o.MaxAge == nil || *o.MaxAge != 7 {
				t.Errorf("time_range=week → MaxAge %+v", o.MaxAge)
			}
		}},
	}
	for _, tc := range tests {
		if _, _ = serve(t, svc, tc.target); svc.gotOpts.Query != "x" {
			t.Fatalf("query not propagated for %s", tc.target)
		}
		tc.check(t, svc.gotOpts)
	}

	// engines= filter drops non-matching results after fan-out.
	code, body := serve(t, svc, "/search?q=x&engines=keepme")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	var results []compatResult
	if err := json.Unmarshal(body["results"], &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Engine != "keepme" {
		t.Fatalf("engines filter leaked: %+v", results)
	}
}

func TestErrorPaths(t *testing.T) {
	if code, _ := serve(t, &stubReporter{}, "/search"); code != 400 {
		t.Errorf("missing q → %d, want 400", code)
	}
	if code, _ := serve(t, &stubReporter{}, "/search?q=x&format=html"); code != 400 {
		t.Errorf("format=html → %d, want 400", code)
	}
	svc := &stubReporter{err: context.DeadlineExceeded}
	code, body := serve(t, svc, "/search?q=x")
	if code != 503 {
		t.Errorf("backend error → %d, want 503", code)
	}
	if msg, _ := body["error"]; !strings.Contains(string(msg), "deadline") {
		t.Errorf("error body = %s", msg)
	}
	health := httptest.NewRecorder()
	NewHandler(&stubReporter{}).ServeHTTP(health, httptest.NewRequest("GET", "/healthz", nil))
	if health.Code != 200 {
		t.Errorf("/healthz → %d", health.Code)
	}
}

func TestSplitURL(t *testing.T) {
	got := splitURL("https://example.com/path;param?a=b#c")
	want := []string{"https", "example.com", "/path", "param", "a=b", "c"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("splitURL = %v, want %v", got, want)
	}
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	_ = sorted
}
