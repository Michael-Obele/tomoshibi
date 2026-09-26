// Package compat serves SearXNG's JSON search API contract so Tomoshibi's
// native engine layer is a drop-in swap for a SearXNG instance:
//
//	GET /search?q=…&format=json&categories=…&pageno=…&time_range=…
//
// The response shape is pinned by golden fixtures captured from the live
// SearXNG container (testdata/*.json): top-level keys query, results,
// answers, corrections, infoboxes, suggestions, unresponsive_engines;
// per-result keys are a superset of the fixture's element keys.
package compat

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/search"
)

// Reporter is the handler's view of the search backend: native search that
// also reports per-engine outcomes (for unresponsive_engines).
type Reporter interface {
	SearchWithReport(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, search.Report, error)
}

// Handler implements the SearXNG JSON surface.
type Handler struct {
	svc Reporter
}

// NewHandler builds the compat listener handler.
func NewHandler(svc Reporter) *Handler { return &Handler{svc: svc} }

// defaultPageSize matches the captured fixtures (20 results/page).
const defaultPageSize = 20

// compatResult mirrors one SearXNG JSON result element. Every key observed
// in the golden fixtures is emitted (nulls where the upstream has no value)
// so consumers doing strict key checks see a superset of the fixture shape.
type compatResult struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Engine        string   `json:"engine"`
	Engines       []string `json:"engines"`
	Template      string   `json:"template"`
	ParsedURL     []string `json:"parsed_url"`
	Category      string   `json:"category"`
	ImgSrc        string   `json:"img_src"`
	Thumbnail     string   `json:"thumbnail"`
	PublishedDate *string  `json:"publishedDate"`
	Pubdate       string   `json:"pubdate"`
	Score         float64  `json:"score"`
	Positions     []int    `json:"positions"`
	Priority      string   `json:"priority"`
	Metadata      string   `json:"metadata"`
	Author        string   `json:"author"`
	Views         string   `json:"views"`
	Length        *int     `json:"length"`
	CloseGroup    bool     `json:"close_group"`
	OpenGroup     bool     `json:"open_group"`
	AudioSrc      string   `json:"audio_src"`
	IframeSrc     string   `json:"iframe_src"`
	Homepage      *string  `json:"homepage"`
	LicenseName   *string  `json:"license_name"`
	LicenseURL    *string  `json:"license_url"`
	Maintainer    string   `json:"maintainer"`
	PackageName   string   `json:"package_name"`
	Popularity    *int     `json:"popularity"`
	SourceCodeURL string   `json:"source_code_url"`
	Tags          []string `json:"tags"`
}

// compatResponse is the top-level envelope (exactly the fixture's keys).
type compatResponse struct {
	Query              string         `json:"query"`
	Results            []compatResult `json:"results"`
	Answers            []any          `json:"answers"`
	Corrections        []any          `json:"corrections"`
	Infoboxes          []any          `json:"infoboxes"`
	Suggestions        []any          `json:"suggestions"`
	UnresponsiveEngine [][2]string    `json:"unresponsive_engines"`
}

// ServeHTTP routes /search and /healthz; anything else is 404.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	case "/search":
		h.handleSearch(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	if query == "" {
		writeErr(w, http.StatusBadRequest, "missing query parameter q")
		return
	}
	if f := q.Get("format"); f != "" && f != "json" {
		writeErr(w, http.StatusBadRequest, "only format=json is supported")
		return
	}

	limit := defaultPageSize
	if v := q.Get("pageno"); v != "" {
		if _, err := strconv.Atoi(v); err != nil || v[0] == '-' {
			writeErr(w, http.StatusBadRequest, "invalid pageno")
			return
		}
	}
	pageno, _ := strconv.Atoi(q.Get("pageno"))
	if pageno < 1 {
		pageno = 1
	}
	opts := search.SearchOptions{
		Query:  query,
		Offset: (pageno - 1) * limit,
		Limit:  limit,
	}
	// categories: comma-separated SearXNG names; "it" maps to our "code".
	if cats := q.Get("categories"); cats != "" {
		cat := strings.TrimSpace(strings.Split(cats, ",")[0])
		switch cat {
		case "it", "code":
			cat = "code"
		}
		if !search.IsValidCategory(cat) {
			writeErr(w, http.StatusBadRequest, "invalid category "+cat)
			return
		}
		opts.Category = cat
	} else if cats := q.Get("category"); cats != "" {
		opts.Category = strings.TrimSpace(cats)
	}
	switch q.Get("time_range") {
	case "day":
		d := 1
		opts.MaxAge = &d
	case "week":
		d := 7
		opts.MaxAge = &d
	case "month":
		d := 30
		opts.MaxAge = &d
	}

	results, _, report, err := h.svc.SearchWithReport(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// engines=… is advisory: filter after fan-out (architecture §6).
	only := engineFilter(q.Get("engines"))
	out := make([]compatResult, 0, len(results))
	category := opts.Category
	if category == "" {
		category = "general"
	}
	for i, res := range results {
		if only != nil && !only[res.Engine] {
			continue
		}
		out = append(out, toCompat(res, i+1, category))
	}

	unresp := report.Unresponsive()
	if only != nil {
		filtered := unresp[:0]
		for _, u := range unresp {
			if only[u[0]] {
				filtered = append(filtered, u)
			}
		}
		unresp = filtered
	}
	if unresp == nil {
		unresp = [][2]string{}
	}

	resp := compatResponse{
		Query:              query,
		Results:            out,
		Answers:            []any{},
		Corrections:        []any{},
		Infoboxes:          []any{},
		Suggestions:        []any{},
		UnresponsiveEngine: unresp,
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		// Header already sent; nothing else to do but stop writing.
		return
	}
}

func engineFilter(raw string) map[string]bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	m := map[string]bool{}
	for _, name := range strings.Split(raw, ",") {
		if n := strings.TrimSpace(name); n != "" {
			m[n] = true
		}
	}
	return m
}

func toCompat(res search.Result, position int, category string) compatResult {
	engine := res.Engine
	if engine == "" {
		engine = "native"
	}
	c := compatResult{
		URL:       res.URL,
		Title:     res.Title,
		Content:   res.Description,
		Engine:    engine,
		Engines:   []string{engine},
		Template:  "default.html",
		ParsedURL: splitURL(res.URL),
		Category:  category,
		Score:     res.Relevance,
		Positions: []int{position},
	}
	if res.PublishedAt != nil {
		ts := res.PublishedAt.UTC()
		rfc3339 := ts.Format(time.RFC3339)
		c.PublishedDate = &rfc3339
		c.Pubdate = ts.Format("2006-01-02 15:04:05+0000")
	}
	return c
}

// splitURL mimics SearXNG's parsed_url: the six urlparse components
// (scheme, netloc, path, params, query, fragment) — verified against the
// live instance: ['https','host','/path',”,”,”].
func splitURL(raw string) []string {
	out := [6]string{}
	rest := raw
	if i := strings.Index(rest, "://"); i >= 0 && i < 16 {
		out[0] = rest[:i]
		rest = rest[i+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		out[1] = rest[:i]
		rest = rest[i:]
	} else {
		out[1] = rest
		rest = ""
	}
	if i := strings.Index(rest, "#"); i >= 0 {
		out[5] = rest[i+1:]
		rest = rest[:i]
	}
	if i := strings.Index(rest, "?"); i >= 0 {
		out[4] = rest[i+1:]
		rest = rest[:i]
	}
	if i := strings.Index(rest, ";"); i >= 0 {
		out[3] = rest[i+1:]
		rest = rest[:i]
	}
	out[2] = rest
	return out[:]
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
