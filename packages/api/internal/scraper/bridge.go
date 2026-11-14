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
// signed-in browser, driven by a recipe that names the borrow lane. It is the
// only path that carries a real session, so its results are marked untrusted
// and are never cached (the content behind a login is per-user, and the
// bridge's own pacing rules exist for a reason).
type BridgeTransport struct {
	server   *bridge.Server
	registry *recipe.Registry

	// paceMu guards gates, the per-recipe floor between two runs: the borrow
	// lane drives a real person's browser, so hammering a site is what gets
	// that person's session flagged.
	paceMu sync.Mutex
	gates  map[string]*recipeGate
}

// DefaultBorrowIntervalMS is the floor between two runs of the same recipe when
// the recipe does not set one. Zero disables it, deliberately.
const DefaultBorrowIntervalMS = 2000

// recipeGate is one recipe's pacing state; its mutex is held across the wait.
type recipeGate struct {
	mu   sync.Mutex
	last time.Time
}

// NewBridgeTransport wires the transport over a running bridge server and the
// installed recipe registry.
func NewBridgeTransport(server *bridge.Server, registry *recipe.Registry) *BridgeTransport {
	return &BridgeTransport{server: server, registry: registry, gates: map[string]*recipeGate{}}
}

// ResolveRecipe finds the recipe for a request: an explicit id must exist, and
// otherwise the host picks the most specific match. A nil recipe means "no
// recipe covers this host", which is the plain lane.
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

// Scrape runs one borrow-lane fetch: map the recipe and request actions onto
// the bridge protocol, hand them to the browser, and turn the returned DOM
// into the standard scrape result.
func (t *BridgeTransport) Scrape(ctx context.Context, rawURL string, rec *recipe.Recipe, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	// The borrow lane is an outbound fetch path like any other, so it goes
	// through the same SSRF guard, and before anything else: a private target is
	// refused for that reason even when the bridge itself is down. Without the
	// guard, an API-key holder (or an agent talked into it) could point the
	// user's browser at their LAN or loopback and read the result back. A recipe
	// may cover its own declared hosts with access.allow_private, which is the
	// only way past the guard.
	if host := hostOf(rawURL); !(rec.Access.AllowPrivate && rec.MatchesHost(host)) {
		if err := safeurl.Check(ctx, rawURL); err != nil {
			return nil, fmt.Errorf("recipe %s: %w", rec.ID, err)
		}
	}

	if t == nil || t.server == nil {
		return nil, fmt.Errorf("recipe %s: borrow lane requires the bridge, which is not configured", rec.ID)
	}

	// Politeness: honour the recipe's cadence before touching the user's
	// browser again. The gate is serialised per recipe and stamped when the run
	// actually starts, so simultaneous callers queue instead of waking together.
	// A recipe that says nothing gets the default floor: the borrow lane drives
	// a real person's session, and an unconfigured pack is not a licence to
	// hammer the site. `min_interval_ms: 0` is the explicit way to opt out.
	minInterval := DefaultBorrowIntervalMS
	if rec.Access.Pace != nil {
		minInterval = rec.Access.Pace.MinIntervalMS
	}
	if minInterval > 0 {
		if err := t.timeGate(ctx, rec.ID, time.Duration(minInterval)*time.Millisecond); err != nil {
			return nil, err
		}
	}

	// The bridge lane returns the page as the browser saw it; the enrichment
	// tail (summary, PII redaction, image extraction) runs on the markdown/HTML
	// path only. Refuse loudly rather than accept an option and drop it.
	if opts.Images {
		return nil, fmt.Errorf("recipe %s: images are not supported on the borrow lane yet", rec.ID)
	}

	actions := make([]bridge.Action, 0, len(rec.Request.Actions)+len(opts.Actions))
	for _, a := range rec.Request.Actions {
		actions = append(actions, bridge.Action{
			Type: a.Type, Selector: a.Selector, Text: a.Text, Key: a.Key,
			URL: a.URL, Script: a.Script, Ms: a.Ms, Px: a.Px,
		})
	}
	for _, a := range opts.Actions {
		actions = append(actions, bridge.Action{
			Type: a.Type, Selector: a.Selector, Script: a.Script, Ms: a.Ms,
		})
	}

	timeout := rec.Request.TimeoutMS
	if timeout <= 0 {
		timeout = 60000
	}
	fullPage := true
	if opts.ScreenshotOpts != nil && opts.ScreenshotOpts.FullPage != nil {
		fullPage = *opts.ScreenshotOpts.FullPage
	}

	req := bridge.FetchRequest{
		URL:          rawURL,
		Actions:      actions,
		WaitSelector: rec.Request.WaitSelector,
		SettleMS:     rec.Request.SettleMS,
		TimeoutMS:    timeout,
		Screenshot:   opts.Screenshot,
		FullPage:     fullPage,
		IO:           rec.IO(),
		Recipe:       rec.ID,
		Lane:         rec.Access.Lane,
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
			return nil, fmt.Errorf("recipe %s: %w", rec.ID, err)
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
		return nil, fmt.Errorf("recipe %s: the bridge is paused in the extension popup; unpause it and retry", rec.ID)
	default:
		return nil, fmt.Errorf("recipe %s: bridge reported %q: %s", rec.ID, res.Status, res.Error)
	}
	if res.HTML == "" {
		return nil, fmt.Errorf("recipe %s: the browser returned no HTML for %s", rec.ID, res.FinalURL)
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
	if err := checkRecipeExpectations(rec, &domain.ScrapeResult{
		URL: rawURL, Markdown: res.Text, HTML: res.HTML,
	}); err != nil {
		return nil, err
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
	if len(rec.Extract.Schema) > 0 {
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
			logger.Log.Warn("Schema extraction failed", "recipe", rec.ID, "error", extErr)
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

	result.Provenance = &domain.Provenance{
		Transport: "bridge", Lane: rec.Access.Lane, Recipe: rec.Ref(),
		IO: res.IO, Actions: len(res.ActionsRun), Markers: res.Markers,
		Pages: 1, Ms: res.Ms, Untrusted: true,
	}
	for _, a := range res.ActionsRun {
		if !a.OK {
			detail := strings.TrimSpace(a.Type + ": " + a.Error)
			result.Provenance.ActionsFailed = append(result.Provenance.ActionsFailed, detail)
		}
	}
	return result, nil
}

// timeGate holds the caller until the recipe's min_interval_ms floor has passed
// since the previous run STARTED, and stamps this run. The per-recipe mutex is
// held across the wait on purpose: two callers arriving together must queue
// rather than wake at the same instant, which is the burst the floor exists to
// prevent.
func (t *BridgeTransport) timeGate(ctx context.Context, recipeID string, min time.Duration) error {
	t.paceMu.Lock()
	gate, ok := t.gates[recipeID]
	if !ok {
		gate = &recipeGate{}
		t.gates[recipeID] = gate
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
