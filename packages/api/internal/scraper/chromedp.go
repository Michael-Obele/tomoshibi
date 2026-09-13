package scraper

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// defaultRecycleAfter is the default number of scrapes before the Chrome
// allocator is restarted to bound long-term memory growth.
const defaultRecycleAfter = 100

// ChromedpScraper reuses a single Chrome allocator across requests, spawning
// lightweight tabs per scrape, and restarts the allocator periodically to
// prevent memory leaks.
type ChromedpScraper struct {
	mu           sync.Mutex
	allocCtx     context.Context
	cancel       context.CancelFunc
	scrapeCount  int
	recycleAfter int
	newAllocator func() (context.Context, context.CancelFunc)
}

// NewChromedpScraper creates a scraper with the default recycle threshold.
func NewChromedpScraper() *ChromedpScraper {
	return NewChromedpScraperWithLimit(defaultRecycleAfter)
}

// NewChromedpScraperWithLimit creates a scraper that restarts its Chrome
// allocator after recycleAfter scrapes. Values <= 0 fall back to the default.
func NewChromedpScraperWithLimit(recycleAfter int) *ChromedpScraper {
	if recycleAfter <= 0 {
		recycleAfter = defaultRecycleAfter
	}
	s := &ChromedpScraper{
		recycleAfter: recycleAfter,
		newAllocator: buildAllocator,
	}
	s.allocCtx, s.cancel = s.newAllocator()
	warmUp(s.allocCtx)
	return s
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
// user-supplied ScreenshotOptions. A nil opts yields jpeg @1920x1080 q90.
func resolveScreenshotParams(opts *domain.ScreenshotOptions) screenshotParams {
	p := screenshotParams{width: 1920, height: 1080, format: "jpeg", quality: 90}
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
	p.fullPage = opts.FullPage
	p.waitSelector = opts.WaitSelector
	return p
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

	var htmlContent string
	screenshotBuf := []byte{}

	logger.Log.Info("Chromedp Scraping", "url", url, "screenshot", opts.Screenshot)

	// Navigate first, then run any requested page actions, then capture.
	err := chromedp.Run(taskCtx,
		emulation.SetUserAgentOverride(gofakeit.UserAgent()),
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
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

	// Use Evaluate instead of OuterHTML to avoid stale node references
	// on SPAs that replace the DOM after the initial page load.
	if err := chromedp.Run(taskCtx, chromedp.Evaluate(`document.documentElement.outerHTML`, &htmlContent)); err != nil {
		return nil, fmt.Errorf("chromedp HTML capture failed: %w", err)
	}

	// If screenshot is requested, do it in a separate Run call so the
	// viewport resize doesn't invalidate the HTML node references from
	// the navigation action above.
	if opts.Screenshot {
		p := resolveScreenshotParams(opts.ScreenshotOpts)

		actions := []chromedp.Action{}
		if p.waitSelector != "" {
			actions = append(actions, chromedp.WaitVisible(p.waitSelector, chromedp.ByQuery))
		}
		actions = append(actions,
			chromedp.EmulateViewport(int64(p.width), int64(p.height)),
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				screenshotBuf, err = page.CaptureScreenshot().
					WithFormat(page.CaptureScreenshotFormat(p.format)).
					WithQuality(int64(p.quality)).
					WithCaptureBeyondViewport(p.fullPage).
					Do(ctx)
				return err
			}),
		)
		err = chromedp.Run(taskCtx, actions...)
		if err != nil {
			// Log screenshot failure but don't fail the whole scrape
			logger.Log.Warn("Screenshot failed, returning HTML-only result", "url", url, "error", err)
		}
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
		result.Screenshot = &domain.ScreenshotData{
			Blob:       base64.StdEncoding.EncodeToString(screenshotBuf),
			Format:     "jpeg",
			SizeBytes:  int64(len(screenshotBuf)),
			CapturedAt: time.Now(),
		}
	}

	return result, nil
}
