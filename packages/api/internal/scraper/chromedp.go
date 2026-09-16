package scraper

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"sync"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// defaultRecycleAfter is the default number of scrapes before the Chrome
// allocator is restarted to bound long-term memory growth.
const defaultRecycleAfter = 100

// Screenshot capture tuning. A capture is only attempted after the page has
// reported itself loaded and its network traffic has gone quiet, because a
// JS app that hydrates after the load event looks empty otherwise.
const (
	// defaultScreenshotMaxHeight caps full-page screenshot height in CSS
	// pixels. Chromium historically hard-capped captures around 16384px, and
	// the cap also bounds the render surface on small hosts.
	// Override per deployment with APP_SCREENSHOT_MAX_HEIGHT.
	defaultScreenshotMaxHeight = 16384
	// maxScreenshotMaxHeight is the largest cap we accept; taller surfaces
	// grow large enough to threaten a 512MB host.
	maxScreenshotMaxHeight = 32768

	// screenshotLoadTimeout bounds the initial wait for load + network idle.
	screenshotLoadTimeout = 15 * time.Second
	// screenshotPostScrollTimeout bounds the wait after the lazy-load scroll.
	screenshotPostScrollTimeout = 10 * time.Second
	// networkIdleQuiet is how long the network must stay silent to count as idle.
	networkIdleQuiet = 500 * time.Millisecond
	// readinessPollInterval is the polling cadence while waiting for the page.
	readinessPollInterval = 100 * time.Millisecond
	// screenshotSettleDelay is a final pause so the last paint lands.
	screenshotSettleDelay = 300 * time.Millisecond
	// autoScrollMaxSteps bounds the lazy-load scroll pass.
	autoScrollMaxSteps = 40
	// autoScrollSettle is the pause after each scroll step.
	autoScrollSettle = 150 * time.Millisecond
)

// ChromedpScraper reuses a single Chrome allocator across requests, spawning
// lightweight tabs per scrape, and restarts the allocator periodically to
// prevent memory leaks.
type ChromedpScraper struct {
	mu                  sync.Mutex
	allocCtx            context.Context
	cancel              context.CancelFunc
	scrapeCount         int
	recycleAfter        int
	screenshotMaxHeight int
	newAllocator        func() (context.Context, context.CancelFunc)
}

// NewChromedpScraper creates a scraper with the default recycle threshold.
func NewChromedpScraper() *ChromedpScraper {
	return NewChromedpScraperWithLimit(defaultRecycleAfter)
}

// NewChromedpScraperWithLimit creates a scraper that restarts its Chrome
// allocator after recycleAfter scrapes. Values <= 0 fall back to the default.
func NewChromedpScraperWithLimit(recycleAfter int) *ChromedpScraper {
	return NewChromedpScraperWithConfig(recycleAfter, defaultScreenshotMaxHeight)
}

// NewChromedpScraperWithConfig is NewChromedpScraperWithLimit plus an
// explicit full-page screenshot height cap in CSS pixels. Values <= 0 use the
// default cap; values above the supported maximum are clamped.
func NewChromedpScraperWithConfig(recycleAfter, screenshotMaxHeight int) *ChromedpScraper {
	if recycleAfter <= 0 {
		recycleAfter = defaultRecycleAfter
	}
	s := &ChromedpScraper{
		recycleAfter:        recycleAfter,
		screenshotMaxHeight: clampScreenshotMaxHeight(screenshotMaxHeight),
		newAllocator:        buildAllocator,
	}
	s.allocCtx, s.cancel = s.newAllocator()
	warmUp(s.allocCtx)
	return s
}

// clampScreenshotMaxHeight keeps a configured cap inside the supported range.
func clampScreenshotMaxHeight(px int) int {
	if px <= 0 {
		return defaultScreenshotMaxHeight
	}
	if px > maxScreenshotMaxHeight {
		return maxScreenshotMaxHeight
	}
	return px
}

// capturedAllocatorFlags returns the stealth flag names baked into
// buildAllocator. It is a pure function returning a literal so tests do not
// depend on mutable global state and remain race-free under -race.
func capturedAllocatorFlags() []string {
	return []string{
		"headless",
		"disable-gpu",
		"no-sandbox",
		"disable-dev-shm-usage",
		"disable-blink-features",
		"disable-infobars",
		"excludeSwitches",
	}
}

// browserFetcher is the minimal interface satisfied by ChromedpScraper.FetchHTML.
// Defined locally to avoid an import cycle with internal/search.
type browserFetcher interface {
	FetchHTML(context.Context, string) (string, error)
}

var _ browserFetcher = (*ChromedpScraper)(nil)

// buildAllocator constructs a fresh Chrome exec allocator with stealth flags.
func buildAllocator() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("excludeSwitches", "enable-automation"),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"),
	)

	// Respect CHROME_BIN env var if set (Dockerfile sets it)
	if chromeBin := os.Getenv("CHROME_BIN"); chromeBin != "" {
		if _, err := os.Stat(chromeBin); err == nil {
			opts = append(opts, chromedp.ExecPath(chromeBin))
			logger.Log.Info("Chrome binary found", "path", chromeBin)
		} else {
			logger.Log.Warn("CHROME_BIN set but binary not found", "path", chromeBin, "error", err)
		}
	}

	return chromedp.NewExecAllocator(context.Background(), opts...)
}

// stealthFlagsContain reports whether the allocator flags contain the given substring.
func stealthFlagsContain(flag string) bool {
	joined := strings.Join(capturedAllocatorFlags(), ",")
	return strings.Contains(joined, flag)
}

// warmUp starts the browser synchronously so failures surface at startup.
func warmUp(allocCtx context.Context) {
	warmCtx, warmCancel := chromedp.NewContext(allocCtx)
	defer warmCancel()
	timedCtx, cancelTimeout := context.WithTimeout(warmCtx, 15*time.Second)
	defer cancelTimeout()
	if err := chromedp.Run(timedCtx); err != nil {
		logger.Log.Warn("Chrome browser not available — dynamic rendering disabled", "error", err)
	} else {
		logger.Log.Info("Chrome browser started successfully (dynamic rendering enabled)")
	}
}

// shouldRecycle reports whether the scrape counter crossed the threshold.
func shouldRecycle(count, after int) bool {
	return after > 0 && count >= after
}

// beginScrape atomically bumps the scrape counter, recycling the allocator
// when the threshold is crossed, and returns the current allocator context.
func (s *ChromedpScraper) beginScrape() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrapeCount++
	if shouldRecycle(s.scrapeCount, s.recycleAfter) {
		s.recycleLocked()
	}
	return s.allocCtx
}

// recycleLocked cancels the old allocator and swaps in a fresh one. Callers
// must hold s.mu. The new browser spawns lazily on the next tab use.
func (s *ChromedpScraper) recycleLocked() {
	if s.cancel != nil {
		s.cancel()
	}
	s.allocCtx, s.cancel = s.newAllocator()
	s.scrapeCount = 0
	logger.Log.Info("Chrome allocator recycled to bound memory growth")
}

// FetchHTML fetches the HTML for the given URL in a lightweight tab, with
// per-request User-Agent rotation and stealth allocator flags. It validates
// the URL via safeurl.Check, respects context cancellation and deadline
// (30s default or caller's deadline), and uses chromedp.Navigate +
// WaitVisible("body") + OuterHTML("html") to retrieve the rendered HTML.
func (s *ChromedpScraper) FetchHTML(ctx context.Context, url string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := safeurl.Check(ctx, url); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	tabCtx, cancel := chromedp.NewContext(s.beginScrape())
	defer cancel()

	timeout := 30 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
		if timeout <= 0 {
			return "", ctx.Err()
		}
	}

	tabCtx, cancelTimeout := context.WithTimeout(tabCtx, timeout)
	defer cancelTimeout()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			cancelTimeout()
		case <-done:
		}
	}()

	ua := gofakeit.UserAgent()
	var html string
	err := chromedp.Run(tabCtx,
		emulation.SetUserAgentOverride(ua),
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("fetch html: %w", err)
	}
	return html, nil
}

// maxScrollSettleIterations bounds the scroll_to_bottom settle loop.
const maxScrollSettleIterations = 10

// buildActionSteps converts a page action into chromedp steps.
func buildActionSteps(a domain.Action) ([]chromedp.Action, error) {
	switch a.Type {
	case "wait_ms":
		ms := a.Ms
		if ms <= 0 {
			ms = 500
		}
		return []chromedp.Action{chromedp.Sleep(time.Duration(ms) * time.Millisecond)}, nil
	case "wait_selector":
		if a.Selector == "" {
			return nil, fmt.Errorf("wait_selector requires a selector")
		}
		return []chromedp.Action{chromedp.WaitVisible(a.Selector, chromedp.ByQuery)}, nil
	case "click":
		if a.Selector == "" {
			return nil, fmt.Errorf("click requires a selector")
		}
		return []chromedp.Action{chromedp.Click(a.Selector, chromedp.NodeVisible)}, nil
	case "scroll_down":
		return []chromedp.Action{chromedp.Evaluate(`window.scrollBy(0, window.innerHeight)`, nil)}, nil
	case "scroll_to_bottom":
		// Scroll with a settle loop so lazy-loaded content has time to
		// render before capture.
		return []chromedp.Action{chromedp.ActionFunc(func(ctx context.Context) error {
			for range maxScrollSettleIterations {
				var scrollY, scrollHeight int
				if err := chromedp.Evaluate(`window.scrollY`, &scrollY).Do(ctx); err != nil {
					return err
				}
				if err := chromedp.Evaluate(`document.body.scrollHeight`, &scrollHeight).Do(ctx); err != nil {
					return err
				}
				if scrollY >= scrollHeight {
					return nil
				}
				if err := chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil).Do(ctx); err != nil {
					return err
				}
				select {
				case <-time.After(500 * time.Millisecond):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})}, nil
	default:
		return nil, fmt.Errorf("unknown action type %q", a.Type)
	}
}

func (s *ChromedpScraper) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// screenshotParams is the resolved, validated set of screenshot settings.
type screenshotParams struct {
	width        int
	height       int
	format       string // "jpeg" or "png"
	quality      int
	fullPage     bool
	waitSelector string
}

// resolveScreenshotParams applies defaults and clamps values from the
// user-supplied ScreenshotOptions. A nil opts yields jpeg @1920x1080 q90,
// full page; full-page capture is the default and must be opted out of with
// an explicit full_page=false.
func resolveScreenshotParams(opts *domain.ScreenshotOptions) screenshotParams {
	p := screenshotParams{width: 1920, height: 1080, format: "jpeg", quality: 90, fullPage: true}
	if opts == nil {
		return p
	}
	if opts.Width > 0 {
		p.width = opts.Width
	}
	if opts.Height > 0 {
		p.height = opts.Height
	}
	switch opts.Format {
	case "png":
		p.format = "png"
	case "jpeg", "jpg", "":
		p.format = "jpeg"
	default:
		p.format = "jpeg"
	}
	if opts.Quality > 0 && opts.Quality <= 100 {
		p.quality = opts.Quality
	}
	if opts.FullPage != nil {
		p.fullPage = *opts.FullPage
	}
	p.waitSelector = opts.WaitSelector
	return p
}

// prepareScreenshot runs the settle sequence before a capture: wait for the
// document to finish loading, then — for full-page captures — scroll through
// the page so lazy content loads, and wait again for the traffic the scroll
// kicked off. The final short sleep lets the last paint land.
func (s *ChromedpScraper) prepareScreenshot(ctx context.Context, p screenshotParams, tracker *requestTracker) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := waitForPageReady(ctx, tracker, screenshotLoadTimeout); err != nil {
			return err
		}
		if p.fullPage {
			if err := autoScroll(ctx); err != nil {
				return err
			}
			if err := waitForPageReady(ctx, tracker, screenshotPostScrollTimeout); err != nil {
				return err
			}
		}
		return sleepCtx(ctx, screenshotSettleDelay)
	}))
}

// takeScreenshot captures the page. Failures are logged and reported as an
// empty capture — a scrape must not fail because its screenshot did.
func (s *ChromedpScraper) takeScreenshot(ctx context.Context, p screenshotParams, url string) ([]byte, bool) {
	actions := []chromedp.Action{}
	if p.waitSelector != "" {
		actions = append(actions, chromedp.WaitVisible(p.waitSelector, chromedp.ByQuery))
	}

	var buf []byte
	var truncated bool
	actions = append(actions,
		chromedp.EmulateViewport(int64(p.width), int64(p.height)),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, truncated, err = capturePage(ctx, p, s.screenshotMaxHeight)
			return err
		}),
	)
	if err := chromedp.Run(ctx, actions...); err != nil {
		logger.Log.Warn("Screenshot failed, returning HTML-only result", "url", url, "error", err)
		return nil, false
	}
	return buf, truncated
}

// capturePage takes the actual screenshot. Viewport captures go straight
// through; full-page captures read the document's content size first, so the
// height can be clamped to maxHeight and reported as truncated.
func capturePage(ctx context.Context, p screenshotParams, maxHeight int) ([]byte, bool, error) {
	shot := page.CaptureScreenshot().
		WithFormat(page.CaptureScreenshotFormat(p.format)).
		WithQuality(int64(p.quality))
	if !p.fullPage {
		buf, err := shot.Do(ctx)
		return buf, false, err
	}

	// The CSS values are what a clip expects: content size in CSS pixels.
	// (The first three returns are the deprecated device-pixel variants.)
	_, _, _, _, _, contentSize, err := page.GetLayoutMetrics().Do(ctx)
	if err != nil {
		// Capture unclamped rather than not at all; the cap is an upper
		// bound, not a correctness requirement.
		logger.Log.Warn("Layout metrics unavailable, capturing full page unclamped", "error", err)
		buf, err := shot.WithCaptureBeyondViewport(true).Do(ctx)
		return buf, false, err
	}

	width := int(contentSize.Width)
	height := int(contentSize.Height)
	if width <= 0 {
		width = p.width
	}
	// Never capture less than the viewport the caller asked for: a short
	// page still gets a viewport-sized image.
	if height < p.height {
		height = p.height
	}
	height, truncated := clampScreenshotHeight(height, maxHeight)

	clip := &page.Viewport{X: 0, Y: 0, Width: float64(width), Height: float64(height), Scale: 1}
	buf, err := shot.WithCaptureBeyondViewport(true).WithClip(clip).Do(ctx)
	return buf, truncated, err
}

// clampScreenshotHeight caps a content height at max and reports whether a
// clip had to be applied.
func clampScreenshotHeight(height, max int) (int, bool) {
	if max > 0 && height > max {
		return max, true
	}
	return height, false
}

// screenshotDimensions reads the real pixel size off the encoded image so
// the response reports what was captured, not what was requested.
func screenshotDimensions(buf []byte, p screenshotParams) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return p.width, p.height
	}
	return cfg.Width, cfg.Height
}

// waitForPageReady blocks until the document reports "complete" and the
// network has been quiet for networkIdleQuiet. It returns nil on timeout
// too: a page that never fully settles should still produce a capture.
func waitForPageReady(ctx context.Context, tracker *requestTracker, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var idleSince time.Time
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var state string
		if err := chromedp.Evaluate(`document.readyState`, &state).Do(ctx); err != nil {
			return err
		}

		if tracker != nil && tracker.busy() {
			idleSince = time.Time{}
		} else if idleSince.IsZero() {
			idleSince = time.Now()
		}

		if state == "complete" && !idleSince.IsZero() && time.Since(idleSince) >= networkIdleQuiet {
			return nil
		}
		if time.Now().After(deadline) {
			// Proceed anyway: some pages hold connections open forever
			// (websockets, long-polling) and never look idle.
			logger.Log.Warn("Page did not settle before capture; proceeding with current state",
				"ready_state", state, "network_busy", tracker != nil && tracker.busy(), "waited", timeout)
			return nil
		}
		if err := sleepCtx(ctx, readinessPollInterval); err != nil {
			return err
		}
	}
}

// autoScroll walks the page down one (nearly) full viewport at a time so
// IntersectionObserver-style lazy loaders fire, then returns to the top.
func autoScroll(ctx context.Context) error {
	var pos struct {
		Y  int `json:"y"`
		VH int `json:"vh"`
		SH int `json:"sh"`
	}
	const positionExpr = `({
		y: Math.round(window.scrollY),
		vh: window.innerHeight,
		sh: Math.max(
			document.body ? document.body.scrollHeight : 0,
			document.documentElement ? document.documentElement.scrollHeight : 0
		)
	})`

	for i := 0; i < autoScrollMaxSteps; i++ {
		if err := chromedp.Evaluate(positionExpr, &pos).Do(ctx); err != nil {
			return err
		}
		if pos.Y+pos.VH >= pos.SH {
			break
		}
		step := pos.VH * 9 / 10
		if step < 1 {
			break
		}
		if err := chromedp.Evaluate(fmt.Sprintf(`window.scrollBy(0, %d)`, step), nil).Do(ctx); err != nil {
			return err
		}
		if err := sleepCtx(ctx, autoScrollSettle); err != nil {
			return err
		}
	}

	if err := chromedp.Evaluate(`window.scrollTo(0, 0)`, nil).Do(ctx); err != nil {
		return err
	}
	return sleepCtx(ctx, autoScrollSettle)
}

// sleepCtx sleeps for d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// requestTracker counts in-flight network requests so a screenshot can wait
// for the page to go quiet. Entries are keyed by request ID: redirects fire
// RequestWillBeSent again for the same ID, while LoadingFinished fires once.
type requestTracker struct {
	mu     sync.Mutex
	active map[network.RequestID]struct{}
}

func newRequestTracker() *requestTracker {
	return &requestTracker{active: make(map[network.RequestID]struct{})}
}

func (t *requestTracker) start(id network.RequestID) {
	t.mu.Lock()
	t.active[id] = struct{}{}
	t.mu.Unlock()
}

func (t *requestTracker) finish(id network.RequestID) {
	t.mu.Lock()
	delete(t.active, id)
	t.mu.Unlock()
}

func (t *requestTracker) busy() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.active) > 0
}

// trackRequests attaches a request tracker to the tab. Listeners must be
// registered before the first Run so no events are missed.
func trackRequests(ctx context.Context) *requestTracker {
	t := newRequestTracker()
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			t.start(e.RequestID)
		case *network.EventLoadingFinished:
			t.finish(e.RequestID)
		case *network.EventLoadingFailed:
			t.finish(e.RequestID)
		}
	})
	return t
}

func (s *ChromedpScraper) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	// Chrome makes the connection itself, so the dial-time guard used on our
	// own http.Clients cannot reach it. Validate up front instead. This does
	// not cover in-page redirects or subresources; treat it as a guard on
	// what the caller asked for, not a browser sandbox.
	if err := safeurl.Check(ctx, url); err != nil {
		return nil, err
	}

	// Create a new tab (Context) from the existing allocator
	// This is much faster than starting a new browser process
	taskCtx, cancelTask := chromedp.NewContext(s.beginScrape())
	defer cancelTask()

	// Set a hard timeout for the browser actions
	// Use the parent context's deadline if available, otherwise default to 60s
	// But we must respect the parent context cancellation
	timeout := 60 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}

	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
	defer cancelTimeout()

	// Screenshot requests track network activity so the capture can wait for
	// the page to go quiet; nothing else needs the network domain enabled.
	var tracker *requestTracker
	if opts.Screenshot {
		tracker = trackRequests(taskCtx)
	}

	var htmlContent string
	screenshotBuf := []byte{}
	screenshotTruncated := false
	shotParams := resolveScreenshotParams(opts.ScreenshotOpts)

	logger.Log.Info("Chromedp Scraping", "url", url, "screenshot", opts.Screenshot)

	// Navigate first, then run any requested page actions, then capture.
	navigation := []chromedp.Action{
		emulation.SetUserAgentOverride(gofakeit.UserAgent()),
	}
	// Screenshot requests render at the capture viewport from the start, so
	// responsive breakpoints and lazy-loading decisions match the final image.
	if tracker != nil {
		navigation = append(navigation,
			chromedp.EmulateViewport(int64(shotParams.width), int64(shotParams.height)),
			network.Enable(),
		)
	}
	navigation = append(navigation,
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
	err := chromedp.Run(taskCtx, navigation...)
	if err != nil {
		return nil, fmt.Errorf("chromedp navigation failed: %w", err)
	}

	// Page actions (wait/click/scroll) run before HTML capture so
	// interaction-driven content is included.
	if len(opts.Actions) > 0 {
		steps := []chromedp.Action{}
		for i, a := range opts.Actions {
			as, err := buildActionSteps(a)
			if err != nil {
				return nil, fmt.Errorf("action %d invalid: %w", i, err)
			}
			steps = append(steps, as...)
		}
		if err := chromedp.Run(taskCtx, steps...); err != nil {
			logger.Log.Warn("Page actions failed, continuing with current DOM", "url", url, "error", err)
		}
	}

	// Let the page genuinely finish loading before anything is captured:
	// wait for the document to report complete, for network traffic to go
	// quiet (JS apps keep fetching after the load event), and — for
	// full-page captures — for lazy content below the fold to load.
	if opts.Screenshot {
		if err := s.prepareScreenshot(taskCtx, shotParams, tracker); err != nil {
			logger.Log.Warn("Screenshot preparation failed, capturing anyway", "url", url, "error", err)
		}
	}

	// Use Evaluate instead of OuterHTML to avoid stale node references
	// on SPAs that replace the DOM after the initial page load.
	if err := chromedp.Run(taskCtx, chromedp.Evaluate(`document.documentElement.outerHTML`, &htmlContent)); err != nil {
		return nil, fmt.Errorf("chromedp HTML capture failed: %w", err)
	}

	// If screenshot is requested, do it in a separate Run call so the
	// viewport resize doesn't invalidate the HTML node references from
	// the navigation action above.
	if opts.Screenshot {
		screenshotBuf, screenshotTruncated = s.takeScreenshot(taskCtx, shotParams, url)
	}

	if htmlContent == "" {
		return nil, fmt.Errorf("empty response from browser")
	}

	// Apply cleaner-output defaults (block ads, drop base64 images) before
	// readability strips the class/id/aria-label attributes the selectors need.
	clean := cleanContent(htmlContent,
		opts.BlockAds == nil || *opts.BlockAds,
		opts.RemoveBase64Images == nil || *opts.RemoveBase64Images,
	)

	// Extract the main content after cleaning so nav/ads/footers don't pollute
	// the LLM-ready output. Falls back to full HTML on failure.
	rc, _ := ExtractMainContent(clean, url)

	markdown, err := md.ConvertString(rc.ContentHTML)
	if err != nil {
		return nil, fmt.Errorf("markdown conversion failed: %w", err)
	}

	metadata := map[string]string{
		"scraped_at": time.Now().Format(time.RFC3339),
		"engine":     "chromedp",
	}
	applyReadabilityMetadata(metadata, rc)

	links := []domain.LinkData{}
	if opts.IncludeLinks == nil || *opts.IncludeLinks {
		links = ExtractLinks(rc.ContentHTML, url)
	}

	result := &domain.ScrapeResult{
		URL:      url,
		Markdown: markdown,
		HTML:     htmlContent,
		Metadata: metadata,
		Links:    links,
	}

	if opts.Screenshot && len(screenshotBuf) > 0 {
		width, height := screenshotDimensions(screenshotBuf, shotParams)
		result.Screenshot = &domain.ScreenshotData{
			Blob:       base64.StdEncoding.EncodeToString(screenshotBuf),
			Format:     shotParams.format,
			Width:      width,
			Height:     height,
			FullPage:   shotParams.fullPage,
			Truncated:  screenshotTruncated,
			SizeBytes:  int64(len(screenshotBuf)),
			CapturedAt: time.Now(),
		}
	}

	return result, nil
}
