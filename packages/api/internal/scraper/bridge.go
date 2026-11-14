package scraper

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/bridge"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/extract"
	"github.com/Michael-Obele/tomoshibi/internal/recipe"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// BridgeTransport fetches pages through the browser bridge: the user's own
// signed-in browser. It is the only path that carries a real session, so its
// results are marked untrusted and are never cached (the content behind a login
// is per-user, and the bridge's own pacing rules exist for a reason).
//
// A recipe is optional (D30). Transport is infrastructure; a recipe is the
// opt-in specialization for a site the generic driver cannot handle. Without
// one, the per-origin allowlist is what says yes or no (D31).
type BridgeTransport struct {
	server   *bridge.Server
	registry *recipe.Registry

	// paceMu guards gates, the per-key floor between two runs: the borrow lane
	// drives a real person's browser, so hammering a site is what gets that
	// person's session flagged.
	paceMu sync.Mutex
	gates  map[string]*recipeGate

	// budget caps how many pages may be read per hour. A research loop with a
	// bug should cost a refusal, not a hundred tabs on a real session.
	budgetMu sync.Mutex
	window   []time.Time
}

// DefaultBorrowIntervalMS is the floor between two runs of the same recipe when
// the recipe does not set one. Zero disables it, deliberately.
const DefaultBorrowIntervalMS = 2000

// DefaultPagesPerHour is the borrow-lane page budget. It is generous enough for
// a real research run and still a ceiling: a runaway loop stops instead of
// spending someone's account.
const DefaultPagesPerHour = 600

// maxBridgeActions caps one request's action list. A recipe is data, and data
// can be wrong; 64 steps is far more than any real page needs and still bounds
// what a broken pack can do inside a signed-in tab.
const maxBridgeActions = 64

// recipeGate is one recipe's pacing state; its mutex is held across the wait.
type recipeGate struct {
	mu   sync.Mutex
	last time.Time
}

// NewBridgeTransport wires the transport over a running bridge server and the
// installed recipe registry (which may be nil).
func NewBridgeTransport(server *bridge.Server, registry *recipe.Registry) *BridgeTransport {
	return &BridgeTransport{server: server, registry: registry, gates: map[string]*recipeGate{}}
}

// ResolveRecipe finds the recipe for a request: an explicit id must exist, and
// otherwise the host picks the most specific match. A nil recipe means "no
// recipe covers this host", which is the plain lane — or, with
// transport=bridge, the generic browser driver.
func (t *BridgeTransport) ResolveRecipe(rawURL, explicitID string) (*recipe.Recipe, error) {
	if t == nil || t.registry == nil {
		if explicitID != "" {
			return nil, fmt.Errorf("recipe %q requested but no recipe registry is configured", explicitID)
		}
		return nil, nil
	}
	if explicitID != "" {
		r, ok := t.registry.ByID(explicitID)
		if !ok {
			return nil, fmt.Errorf("recipe %q is not installed (tomoshibi recipes list)", explicitID)
		}
		return r, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return nil, nil
	}
	return t.registry.Match(u.Hostname()), nil
}

// runLabel names the run in errors: the recipe when there is one, the origin
// otherwise. A recipe-free run must still say something a person can act on.
func runLabel(rec *recipe.Recipe, rawURL string) string {
	if rec != nil {
		return "recipe " + rec.ID
	}
	if host := hostOf(rawURL); host != "" {
		return "bridge " + host
	}
	return "bridge"
}

// Scrape runs one borrow-lane fetch: map the recipe (if any) and the request's
// built-ins onto the bridge protocol, hand them to the browser, and turn the
// returned DOM into the standard scrape result.
func (t *BridgeTransport) Scrape(ctx context.Context, rawURL string, rec *recipe.Recipe, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	label := runLabel(rec, rawURL)
	host := hostOf(rawURL)
	origin := originOf(rawURL)

	// The borrow lane is an outbound fetch path like any other, so it goes
	// through the same SSRF guard, and before anything else: a private target is
	// refused for that reason even when the bridge itself is down. Without the
	// guard, an API-key holder (or an agent talked into it) could point the
	// user's browser at their LAN or loopback and read the result back. A recipe
	// may cover its own declared hosts with access.allow_private, which is the
	// only way past the guard.
	if !(rec != nil && rec.Access.AllowPrivate && rec.MatchesHost(host)) {
		if err := safeurl.Check(ctx, rawURL); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
	}

	if t == nil || t.server == nil {
		return nil, fmt.Errorf("%s: the borrow lane requires the bridge, which is not configured", label)
	}

	// A recipe says which hosts it is for. Enforcing that turns the pack into
	// what the plan always claimed it was: the authorization for those hosts,
	// not a key that opens any door. (An auto-matched recipe always matches; only
	// an explicit id naming a foreign host can land here.)
	if rec != nil && host != "" && !rec.MatchesHost(host) {
		return nil, fmt.Errorf("%s does not cover host %s; use a recipe that declares it", label, host)
	}

	// The gate. A recipe is its own consent — installing the pack was the act —
	// and a non-empty allowlist can still narrow it. Without a recipe the
	// allowlist IS the answer, and an origin it does not name is a refusal a
	// human can act on.
	if !t.consent(rec, host) {
		return nil, &bridge.ConsentRequiredError{Origin: origin}
	}

	if !t.takeBudget(time.Now()) {
		return nil, fmt.Errorf(
			"%s: bridge page budget exhausted (%d pages/hour); raise BRIDGE_PAGES_PER_HOUR or wait for the window to clear",
			label, pagesPerHour(),
		)
	}

	// Politeness: honour the recipe's cadence before touching the user's
	// browser again. The gate is serialised per recipe (or per origin when there
	// is no recipe) and stamped when the run actually starts, so simultaneous
	// callers queue instead of waking together. A run that says nothing gets the
	// default floor: the borrow lane drives a real person's session, and an
	// unconfigured request is not a licence to hammer the site.
	paceKey := origin
	if rec != nil {
		paceKey = rec.ID
	}
	minInterval := DefaultBorrowIntervalMS
	if rec != nil && rec.Access.Pace != nil {
		minInterval = rec.Access.Pace.MinIntervalMS
	}
	if minInterval > 0 {
		if err := t.timeGate(ctx, paceKey, time.Duration(minInterval)*time.Millisecond); err != nil {
			return nil, err
		}
	}

	// The bridge lane returns the page as the browser saw it; the enrichment
	// tail (summary, PII redaction, image extraction) runs on the markdown/HTML
	// path only. Refuse loudly rather than accept an option and drop it.
	if opts.Images {
		return nil, fmt.Errorf("%s: images are not supported on the borrow lane yet", label)
	}
	if opts.CaptureNetwork && !usesTurbo(rec) {
		return nil, fmt.Errorf("%s: capture_network needs io: turbo on the borrow lane", label)
	}

	// Actions: the recipe's first (they are the site's own steps), then the
	// caller's, so a one-off override does not have to fork a pack.
	actions := make([]bridge.Action, 0, 8)
	if rec != nil {
		for _, a := range rec.Request.Actions {
			actions = append(actions, bridge.Action{
				Type: a.Type, Selector: a.Selector, Text: a.Text, Key: a.Key,
				URL: a.URL, Script: a.Script, Ms: a.Ms, Px: a.Px,
			})
		}
	}
	for _, a := range opts.Actions {
		actions = append(actions, bridge.Action{
			Type: a.Type, Selector: a.Selector, Script: a.Script, Ms: a.Ms,
		})
	}
	if len(actions) > maxBridgeActions {
		return nil, fmt.Errorf("%s: %d actions is more than the bridge will run in one page (max %d)",
			label, len(actions), maxBridgeActions)
	}

	timeout := 60000
	waitSelector := ""
	settleMS := 0
	fullPage := true
	if rec != nil {
		if rec.Request.TimeoutMS > 0 {
			timeout = rec.Request.TimeoutMS
		}
		waitSelector = rec.Request.WaitSelector
		settleMS = rec.Request.SettleMS
	}
	if opts.ScreenshotOpts != nil && opts.ScreenshotOpts.FullPage != nil {
		fullPage = *opts.ScreenshotOpts.FullPage
	}

	// Settle is the default on the generic path: a fixed sleep is a guess, and a
	// page that is still animating when it is read is the "moving elements"
	// problem this lane exists to solve. A recipe may still pin its own
	// settle_ms instead.
	settle := opts.Settle
	if settle == nil && (rec == nil || settleMS == 0) {
		settle = &domain.SettleOptions{}
	}

	req := bridge.FetchRequest{
		URL:           rawURL,
		Actions:       actions,
		WaitSelector:  waitSelector,
		SettleMS:      settleMS,
		TimeoutMS:     timeout,
		Screenshot:    opts.Screenshot,
		FullPage:      fullPage,
		IO:            ioFor(rec),
		Settle:        settle,
		ScrollCollect: opts.ScrollCollect,
		PageTree:      opts.PageTree,
		PageTreeOpts:  opts.PageTreeOpts,
		Motion:        opts.Motion,
	}
	if rec != nil {
		req.Recipe = rec.ID
		req.Lane = rec.Access.Lane
	} else {
		req.Lane = recipe.LaneBorrow
	}

	res, err := t.server.Fetch(ctx, req)
	if err != nil {
		// The socket may simply be reconnecting (a browser restart, a worker
		// wake-up): one retry, then the honest error.
		if errors.Is(err, bridge.ErrOffline) {
			select {
			case <-time.After(400 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			res, err = t.server.Fetch(ctx, req)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
	}

	// The browser tells us when a human is needed or a wall was hit. Both are
	// reported as blocks: returning challenge text as content is the one
	// failure an agent cannot detect for itself.
	switch res.Status {
	case "ok":
	case "awaiting-human":
		return nil, &BlockedError{URL: rawURL, Vendor: "bridge-handoff", HTML: res.Text,
			Err: fmt.Errorf("the browser needs you: solve the check in the open tab, then retry (markers: %s)", strings.Join(res.Markers, ", "))}
	case "blocked":
		return nil, &BlockedError{URL: rawURL, Vendor: strings.Join(res.Markers, ","), HTML: res.Text,
			Err: fmt.Errorf("the site served a challenge page (%s)", strings.Join(res.Markers, ", "))}
	case "paused":
		return nil, fmt.Errorf("%s: the bridge is paused in the extension popup; unpause it and retry", label)
	default:
		return nil, fmt.Errorf("%s: bridge reported %q: %s", label, res.Status, res.Error)
	}
	if res.HTML == "" {
		return nil, fmt.Errorf("%s: the browser returned no HTML for %s", label, res.FinalURL)
	}

	// The same challenge detector the engine pipeline uses: a block page that
	// arrives as HTTP 200 is exactly the failure an agent cannot spot for
	// itself, and the borrow lane must not be the one place that returns it as
	// content.
	if ch := detectChallenge([]byte(res.HTML), http.StatusOK); ch != nil {
		return nil, &BlockedError{URL: rawURL, Vendor: ch.Vendor, HTML: res.HTML,
			Err: fmt.Errorf("the site served a %s challenge to the browser", ch.Vendor)}
	}

	// Expectations run against what came back, the same way the engine pipeline
	// checks a plain recipe's.
	if rec != nil {
		if err := checkRecipeExpectations(rec, &domain.ScrapeResult{
			URL: rawURL, Markdown: res.Text, HTML: res.HTML,
		}); err != nil {
			return nil, err
		}
	}

	result, err := buildStaticResult(rawURL, res.HTML, opts)
	if err != nil {
		return nil, err
	}
	if opts.Summary {
		result.Summary = extract.Summarize(result.Markdown, result.Metadata["excerpt"], opts.SummarySentences)
	}
	if opts.RedactPII {
		result.Markdown = extract.RedactPII(result.Markdown)
		if result.Summary != "" {
			result.Summary = extract.RedactPII(result.Summary)
		}
	}
	result.Metadata["engine"] = "bridge"
	if res.Title != "" {
		result.Metadata["title"] = res.Title
	}
	if res.FinalURL != "" {
		result.Metadata["final_url"] = res.FinalURL
	}

	// Deterministic extraction: the recipe's schema first, the request's on
	// top so a caller can override a field without editing the recipe.
	if rec != nil && len(rec.Extract.Schema) > 0 {
		extracted, extErr := extract.Apply(res.HTML, rec.Extract.Schema)
		if extErr != nil {
			logger.Log.Warn("Recipe extraction failed", "recipe", rec.ID, "error", extErr)
		} else {
			result.Extracted = extracted
		}
	}
	if len(opts.ExtractSchema) > 0 {
		extracted, extErr := extract.Apply(res.HTML, opts.ExtractSchema)
		if extErr != nil {
			logger.Log.Warn("Schema extraction failed", "run", label, "error", extErr)
		} else {
			if result.Extracted == nil {
				result.Extracted = extracted
			} else {
				for k, v := range extracted {
					result.Extracted[k] = v
				}
			}
		}
	}

	// The named built-ins travel through untouched: they were produced by the
	// page, and reinterpreting them here would be a second implementation.
	result.PageTree = res.Tree
	result.Motion = res.Motion
	result.Collected = res.Collected

	if opts.Screenshot && res.ScreenshotB64 != "" {
		if raw, decErr := base64.StdEncoding.DecodeString(res.ScreenshotB64); decErr == nil {
			shot := &domain.ScreenshotData{
				Blob: res.ScreenshotB64, Format: "png",
				// The scripting layer can only capture the viewport —
				// tabs.captureVisibleTab has no beyond-viewport mode — so the flag
				// reports what was actually captured, not what was asked for.
				FullPage:   fullPage && res.IO == "turbo",
				SizeBytes:  int64(len(raw)),
				CapturedAt: time.Now(), URL: res.FinalURL,
			}
			if cfg, cfgErr := png.DecodeConfig(bytes.NewReader(raw)); cfgErr == nil {
				shot.Width, shot.Height = cfg.Width, cfg.Height
			}
			result.Screenshot = shot
		}
	}

	prov := &domain.Provenance{
		Transport: "bridge", Lane: recipe.LaneBorrow, IO: res.IO,
		Actions: len(res.ActionsRun), Markers: res.Markers,
		Pages: 1, Ms: res.Ms, Untrusted: true,
	}
	if rec != nil {
		prov.Lane = rec.Access.Lane
		prov.Recipe = rec.Ref()
	}
	if res.SettledMS >= 0 {
		prov.SettledMS = res.SettledMS
	}
	for _, a := range res.ActionsRun {
		if !a.OK {
			detail := strings.TrimSpace(a.Type + ": " + a.Error)
			prov.ActionsFailed = append(prov.ActionsFailed, detail)
		}
	}
	result.Provenance = prov
	return result, nil
}

// MayDrive reports whether the allowlist (and, when given, the recipe) permits
// driving this URL. `transport: auto` asks before it escalates into the browser,
// so an unallowed origin never turns a working anonymous fetch into a refusal.
func (t *BridgeTransport) MayDrive(rec *recipe.Recipe, rawURL string) bool {
	if t == nil || t.server == nil {
		return false
	}
	host := hostOf(rawURL)
	if rec != nil && !rec.MatchesHost(host) {
		return false
	}
	return t.consent(rec, host)
}

// consent reports whether this run may drive the target.
//
// Two separate consents, and the rule is that either one is enough because each
// was a deliberate human act:
//
//   - a recipe that declares this host (installing the pack was the act), or
//   - an allowlist entry (the act was `tomoshibi bridge allow`).
//
// The allowlist deliberately does NOT narrow a recipe. A pack is installed on
// purpose and removed on purpose; making it also need an allowlist entry would
// mean two revocations for one decision, and it would break every existing pack
// the moment a user allowed one unrelated host.
func (t *BridgeTransport) consent(rec *recipe.Recipe, host string) bool {
	if rec != nil && rec.MatchesHost(host) {
		return true
	}
	return bridge.MatchesHostPatterns(t.server.AllowedOrigins(), host)
}

// takeBudget records one page read if the hourly window allows it.
func (t *BridgeTransport) takeBudget(now time.Time) bool {
	limit := pagesPerHour()
	if limit <= 0 {
		return true
	}
	t.budgetMu.Lock()
	defer t.budgetMu.Unlock()
	cut := now.Add(-time.Hour)
	kept := t.window[:0]
	for _, ts := range t.window {
		if ts.After(cut) {
			kept = append(kept, ts)
		}
	}
	t.window = kept
	if len(t.window) >= limit {
		return false
	}
	t.window = append(t.window, now)
	return true
}

// pagesPerHour reads the budget at call time, matching the rest of this
// package's env-at-call-time convention. 0 disables the budget.
func pagesPerHour() int {
	raw := strings.TrimSpace(os.Getenv("BRIDGE_PAGES_PER_HOUR"))
	if raw == "" {
		return DefaultPagesPerHour
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return DefaultPagesPerHour
	}
	return n
}

// ioFor picks the input layer from the recipe's declaration; the generic path
// is scripting, which is the layer that works on both browsers.
func ioFor(rec *recipe.Recipe) string {
	if rec != nil && rec.IO() == recipe.IOTurbo {
		return recipe.IOTurbo
	}
	return recipe.IOScripting
}

// usesTurbo reports whether this run will use the debugger layer.
func usesTurbo(rec *recipe.Recipe) bool {
	return rec != nil && rec.IO() == recipe.IOTurbo
}

// timeGate holds the caller until the pace floor has passed since the previous
// run STARTED, and stamps this run. The per-key mutex is held across the wait on
// purpose: two callers arriving together must queue rather than wake at the
// same instant, which is the burst the floor exists to prevent.
func (t *BridgeTransport) timeGate(ctx context.Context, key string, min time.Duration) error {
	t.paceMu.Lock()
	gate, ok := t.gates[key]
	if !ok {
		gate = &recipeGate{}
		t.gates[key] = gate
	}
	t.paceMu.Unlock()

	gate.mu.Lock()
	defer gate.mu.Unlock()
	if wait := min - time.Since(gate.last); wait > 0 && !gate.last.IsZero() {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	gate.last = time.Now()
	return nil
}

// hostOf is the hostname of a URL, or "" when it has none.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// originOf is the scheme://host of a URL, or "" when it has none.
func originOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// FetchRaw drives one page and returns what the browser saw, with no content
// challenge heuristics applied.
//
// The search rung parses the page itself and decides what a challenge is — it
// parses first, then refuses — which is more precise than the generic detector
// that guards the markdown path. A SERP that merely *mentions* "unusual traffic"
// in an inline script is a page with results on it, and refusing it was a real
// false positive: Google's results page tripped the generic check while plainly
// carrying results.
func (t *BridgeTransport) FetchRaw(ctx context.Context, rawURL string, opts domain.ScrapeOptions) (string, error) {
	if t == nil || t.server == nil {
		return "", fmt.Errorf("bridge: not configured (start the API with the bridge enabled)")
	}
	host := hostOf(rawURL)
	if !t.consent(nil, host) {
		return "", &bridge.ConsentRequiredError{Origin: originOf(rawURL)}
	}
	if err := safeurl.Check(ctx, rawURL); err != nil {
		return "", err
	}
	if !t.takeBudget(time.Now()) {
		return "", fmt.Errorf("bridge: page budget exhausted (%d pages/hour); raise BRIDGE_PAGES_PER_HOUR", pagesPerHour())
	}
	// The same politeness floor as any other borrow run: a search loop is still
	// a person's browser talking to someone else's site.
	if err := t.timeGate(ctx, originOf(rawURL), DefaultBorrowIntervalMS*time.Millisecond); err != nil {
		return "", err
	}
	settle := opts.Settle
	if settle == nil {
		settle = &domain.SettleOptions{}
	}
	// Actions are how the caller drives the page before reading it — the search
	// rung uses them to answer a consent wall once. Same cap as any other run.
	actions := make([]bridge.Action, 0, len(opts.Actions))
	for _, a := range opts.Actions {
		actions = append(actions, bridge.Action{
			Type: a.Type, Selector: a.Selector, Script: a.Script, Ms: a.Ms,
		})
	}
	if len(actions) > maxBridgeActions {
		return "", fmt.Errorf("bridge: %d actions is more than the bridge will run in one page (max %d)", len(actions), maxBridgeActions)
	}

	res, err := t.server.Fetch(ctx, bridge.FetchRequest{
		URL:       rawURL,
		Actions:   actions,
		TimeoutMS: 60000,
		IO:        recipe.IOScripting,
		Settle:    settle,
		PageTree:  opts.PageTree,
		Lane:      recipe.LaneBorrow,
	})
	if err != nil {
		return "", err
	}
	if res.Status != "ok" {
		return "", fmt.Errorf("bridge: %s: %s", res.Status, res.Error)
	}
	return res.HTML, nil
}
