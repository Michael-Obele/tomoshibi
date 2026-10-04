package search

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/search/engines"
	"github.com/Michael-Obele/tomoshibi/internal/telemetry"
)

// EngineOutcome records what one engine did during a query — the data
// behind the compat layer's unresponsive_engines and README G6 telemetry.
type EngineOutcome struct {
	Engine   string
	Results  int
	Err      error
	Duration time.Duration
}

// Report aggregates per-engine outcomes for a single SearchWithReport call.
type Report struct {
	Outcomes []EngineOutcome
}

// Unresponsive returns the SearXNG tuple shape [[name, reason], ...] for
// engines that errored (block/not-configured/timeout/HTTP failure).
func (r Report) Unresponsive() [][2]string {
	var out [][2]string
	for _, o := range r.Outcomes {
		if o.Err == nil {
			continue
		}
		reason := o.Err.Error()
		switch {
		case errors.Is(o.Err, engines.ErrBlocked):
			reason = "blocked: " + stripErrPrefix(o.Err, engines.ErrBlocked)
		case errors.Is(o.Err, engines.ErrNotConfigured):
			reason = "not configured"
		}
		out = append(out, [2]string{o.Engine, reason})
	}
	return out
}

func stripErrPrefix(err, target error) string {
	msg := err.Error()
	tgt := target.Error()
	if i := strings.Index(msg, tgt); i >= 0 {
		rest := strings.TrimLeft(msg[i+len(tgt):], ": ")
		if rest != "" {
			return rest
		}
	}
	return msg
}

// NativeService is the in-house fan-out: it queries every engine registered
// for the requested category in parallel, merges and dedupes results, and
// reports per-engine outcomes. It implements Service; use SearchWithReport
// when the caller needs unresponsive-engine detail (the SearXNG-compat
// listener).
type NativeService struct {
	all   []engines.Engine
	byCat map[string][]engines.Engine
	// rec records per-engine outcomes for telemetry (nil = recording off).
	rec telemetry.Recorder
	// maxPerDomain caps how many results one engine may contribute from a
	// single registrable domain, per category. Nil means uncapped.
	maxPerDomain map[string]int
}

// NativeOption configures optional NativeService behaviour.
type NativeOption func(*NativeService)

// fusionCategories are the roster categories a cap can be configured for.
// "default" is a fallback key, not a category.
var fusionCategories = [...]string{"general", "news", "it"}

// WithMaxPerDomain caps per-engine, per-domain contributions before fusion,
// keyed by registry category. The "default" key is resolved into every
// category here, once, so the hot path is a plain map lookup and a category
// with no explicit entry still gets the roster-wide policy rather than
// silently reverting to uncapped.
func WithMaxPerDomain(byCategory map[string]int) NativeOption {
	return func(n *NativeService) {
		if len(byCategory) == 0 {
			return
		}
		resolved := make(map[string]int, len(fusionCategories))
		fallback := byCategory["default"]
		for _, c := range fusionCategories {
			if v, ok := byCategory[c]; ok {
				resolved[c] = v
			} else {
				resolved[c] = fallback
			}
		}
		n.maxPerDomain = resolved
	}
}

// NewNativeService builds a NativeService from ready-made engines (order
// should be merge order — weight desc). Engines are grouped by category;
// "code" queries map onto the "it" group (same mapping SearXNG uses).
func NewNativeService(engs []engines.Engine, opts ...NativeOption) *NativeService {
	n := &NativeService{
		all:   engs,
		byCat: map[string][]engines.Engine{},
	}
	for _, o := range opts {
		o(n)
	}
	for _, e := range engs {
		for _, c := range e.Categories() {
			n.byCat[c] = append(n.byCat[c], e)
		}
	}
	return n
}

// Engines returns every engine in the service (for wiring/inspection).
func (n *NativeService) Engines() []engines.Engine { return n.all }

// SetRecorder installs the telemetry recorder for per-engine outcomes.
// Passing nil switches recording off.
func (n *NativeService) SetRecorder(r telemetry.Recorder) {
	if r == nil {
		r = telemetry.Nop{}
	}
	n.rec = r
}

// recordOutcomes writes one engine event per outcome, joining the search's
// trace id (set by HybridService on the context).
func (n *NativeService) recordOutcomes(ctx context.Context, report Report) {
	if n.rec == nil {
		return
	}
	trace := telemetry.TraceID(ctx)
	for _, o := range report.Outcomes {
		n.rec.RecordEngine(telemetry.EngineEvent{
			TraceID:   trace,
			Engine:    o.Engine,
			Status:    engineStatus(o),
			Detail:    engineDetail(o),
			Results:   o.Results,
			LatencyMS: o.Duration.Milliseconds(),
		})
	}
}

// engineStatus classifies an outcome for the telemetry scorecard.
func engineStatus(o EngineOutcome) string {
	switch {
	case o.Err == nil:
		if o.Results == 0 {
			return "empty"
		}
		return "ok"
	case errors.Is(o.Err, engines.ErrBlocked):
		return "blocked"
	case errors.Is(o.Err, engines.ErrNotConfigured):
		return "not_configured"
	case errors.Is(o.Err, context.Canceled), errors.Is(o.Err, context.DeadlineExceeded):
		return "timeout"
	case strings.Contains(o.Err.Error(), "timeout"):
		return "timeout"
	default:
		return "error"
	}
}

// engineDetail carries the block marker / error text for non-ok outcomes.
func engineDetail(o EngineOutcome) string {
	if o.Err == nil {
		return ""
	}
	if errors.Is(o.Err, engines.ErrBlocked) {
		return stripErrPrefix(o.Err, engines.ErrBlocked)
	}
	return o.Err.Error()
}

// Search implements Service (see SearchWithReport for the full report).
func (n *NativeService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
	res, total, _, err := n.SearchWithReport(ctx, opts)
	return res, total, err
}

// SearchWithReport fans out to the category's engines concurrently.
// It errors only when EVERY engine failed (so HybridService can fall
// through to SearXNG/Stealth/Brave).
func (n *NativeService) SearchWithReport(ctx context.Context, opts SearchOptions) ([]Result, int, Report, error) {
	if err := ValidateCategory(opts.Category); err != nil {
		return nil, 0, Report{}, err
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, 0, Report{}, errors.New("empty query")
	}
	cat := opts.Category
	if cat == "" {
		cat = "general"
	}
	if cat == "code" {
		cat = "it"
	}
	list := n.byCat[cat]
	if len(list) == 0 {
		return nil, 0, Report{}, fmt.Errorf("no engines registered for category %q", cat)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	q := engines.Query{
		Q:              opts.Query,
		Pageno:         pageOf(opts),
		Language:       "en",
		TimeRange:      timeRangeOf(opts.MaxAge),
		IncludeDomains: opts.IncludeDomains,
		ExcludeDomains: opts.ExcludeDomains,
	}

	type outcome struct {
		idx int
		res []engines.Result
		err error
		dur time.Duration
	}
	ch := make(chan outcome, len(list))
	var wg sync.WaitGroup
	for i, e := range list {
		wg.Add(1)
		go func(i int, e engines.Engine) {
			defer wg.Done()
			start := time.Now()
			res, err := e.Search(ctx, q)
			ch <- outcome{idx: i, res: res, err: err, dur: time.Since(start)}
		}(i, e)
	}
	go func() { wg.Wait(); close(ch) }()

	raw := make(map[int]outcome, len(list))
	report := Report{Outcomes: make([]EngineOutcome, 0, len(list))}
	failures := 0
	for o := range ch {
		raw[o.idx] = o
		e := list[o.idx]
		report.Outcomes = append(report.Outcomes, EngineOutcome{
			Engine: e.Name(), Results: len(o.res), Err: o.err, Duration: o.dur,
		})
		if o.err != nil {
			failures++
		}
	}
	n.recordOutcomes(ctx, report)
	if failures == len(list) {
		var first error
		for _, o := range raw {
			if o.err != nil {
				first = o.err
				break
			}
		}
		return nil, 0, report, fmt.Errorf("all %d engines failed (last: %w)", len(list), first)
	}
	if ctx.Err() != nil {
		return nil, 0, report, ctx.Err()
	}
	sort.Slice(report.Outcomes, func(i, j int) bool { return report.Outcomes[i].Engine < report.Outcomes[j].Engine })

	// Reciprocal-rank fusion across engines.
	//
	// The previous scheme scored every hit by (engine weight, position)
	// alone, which meant a URL found independently by six engines scored the
	// same as the same URL found by one: cross-engine agreement — the only
	// real evidence of quality available to a metasearch — was worth nothing.
	// RRF scores a document by the sum of 1/(k+rank) over the engines that
	// surfaced it, so agreement accumulates and no score normalization
	// between heterogeneous upstreams is required.
	// Named `fe` rather than `engines` so it does not shadow the engines
	// package imported by fusion.go.
	fe := make([]fuseEngine, 0, len(list))
	for i, o := range raw {
		fe = append(fe, fuseEngine{
			name:    list[i].Name(),
			weight:  weightFor(i, len(list)),
			results: capPerDomain(o.res, n.maxPerDomain[cat]),
		})
	}
	fused := fuse(fe)

	merged := make([]Result, 0, len(fused))
	for i, r := range fused {
		r.ID = fmt.Sprintf("%s_%d", opts.Query, i)
		if len(r.Highlights) == 0 {
			r.Highlights = extractHighlights(r.Description, opts.Query)
		}
		merged = append(merged, r)
	}

	merged = filterResults(merged, opts)
	total := len(merged)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	if opts.Rerank {
		merged = rerankTFIDF(opts.Query, merged)
	}
	return merged, total, report, nil
}

// filterResults applies domain include/exclude and MaxAge (published-date)
// filters from SearchOptions. Engines without dates ignore MaxAge.
func filterResults(in []Result, opts SearchOptions) []Result {
	if len(opts.IncludeDomains) == 0 && len(opts.ExcludeDomains) == 0 && opts.MaxAge == nil {
		return in
	}
	include := domainSet(opts.IncludeDomains)
	exclude := domainSet(opts.ExcludeDomains)
	out := in[:0]
	for _, r := range in {
		d := strings.ToLower(r.Domain)
		if len(include) > 0 && !include[d] && !suffixMatch(include, d) {
			continue
		}
		if exclude[d] || suffixMatch(exclude, d) {
			continue
		}
		// MaxAge applies to dated results (RSS engines); undated results
		// are kept — their engines have no date signal to filter on.
		if opts.MaxAge != nil && r.PublishedAt != nil {
			if time.Since(*r.PublishedAt) > time.Duration(*opts.MaxAge)*24*time.Hour {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func domainSet(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, d := range list {
		d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(d, "*.")))
		if d != "" {
			m[d] = true
		}
	}
	return m
}

func suffixMatch(set map[string]bool, domain string) bool {
	for d := range set {
		if strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

// weightFor returns an engine's fusion weight, derived from its position in
// the weight-sorted roster.
//
// Engines reach this function already sorted by registry weight descending
// (engines.Specs sorts on weight, then name), so index i is a direct proxy
// for the roster's trust ordering. The Engine interface deliberately does
// not expose Weight — fusion needs the ordering, not the raw number, and
// widening the interface would couple ranking to the registry type.
//
// The weight decays gently rather than using the raw 40-90 registry values:
// scaling to 0..1 keeps a top engine influential while letting three or four
// agreeing lower-weighted engines still outrank one engine's lone opinion.
func weightFor(index, total int) float64 {
	if total <= 1 {
		return 1
	}
	// 1.0 at the top engine, ~0.5 at the bottom of a 14-engine roster.
	span := float64(total - 1)
	return 1.0 - 0.5*(float64(index)/span)
}

// canonicalURL normalizes a URL for dedup (host case, fragment, trailing
// slash, tracking params that never change content).
func canonicalURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimSpace(raw))
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(k, "utm_") || k == "fbclid" || k == "gclid" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	s := u.String()
	if strings.HasSuffix(s, "/") && u.Path != "/" {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}

// pageOf converts offset+limit into a 1-based SearXNG-style page number.
func pageOf(opts SearchOptions) int {
	if opts.Offset <= 0 {
		return 1
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	return opts.Offset/limit + 1
}

func timeRangeOf(maxAge *int) string {
	if maxAge == nil {
		return ""
	}
	switch *maxAge {
	case 1:
		return "day"
	case 7:
		return "week"
	case 30:
		return "month"
	default:
		return ""
	}
}
