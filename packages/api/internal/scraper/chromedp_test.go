package scraper

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestResolveScreenshotParams_Defaults(t *testing.T) {
	p := resolveScreenshotParams(nil)
	if p.width != 1920 || p.height != 1080 {
		t.Errorf("expected default viewport 1920x1080, got %dx%d", p.width, p.height)
	}
	// Full page is the default: asking for a screenshot asks for the page,
	// not for whatever happens to fit the viewport.
	if p.format != "jpeg" || p.quality != 90 || !p.fullPage {
		t.Errorf("unexpected defaults: format=%s quality=%d fullPage=%v", p.format, p.quality, p.fullPage)
	}
}

func TestResolveScreenshotParams_FullPageOptOut(t *testing.T) {
	if p := resolveScreenshotParams(&domain.ScreenshotOptions{}); !p.fullPage {
		t.Error("full page should stay the default when full_page is omitted")
	}
	if p := resolveScreenshotParams(&domain.ScreenshotOptions{FullPage: boolPtr(false)}); p.fullPage {
		t.Error("explicit full_page=false should capture the viewport only")
	}
	if p := resolveScreenshotParams(&domain.ScreenshotOptions{FullPage: boolPtr(true)}); !p.fullPage {
		t.Error("explicit full_page=true should stay full page")
	}
}

func TestResolveScreenshotParams_AppliesAndClamps(t *testing.T) {
	opts := &domain.ScreenshotOptions{
		Width: 800, Height: 600, FullPage: boolPtr(true), Format: "png",
		Quality: 150, WaitSelector: "#app",
	}
	p := resolveScreenshotParams(opts)
	if p.width != 800 || p.height != 600 {
		t.Errorf("expected 800x600, got %dx%d", p.width, p.height)
	}
	if !p.fullPage || p.format != "png" || p.waitSelector != "#app" {
		t.Errorf("unexpected resolved params: %+v", p)
	}
	if p.quality != 90 {
		t.Errorf("quality 150 should clamp to default 90, got %d", p.quality)
	}
}

// boolPtr is defined in content_clean_test.go; screenshots reuse it so
// options can express "unset" distinctly from "explicitly false" — the
// full_page default depends on that difference.

func TestClampScreenshotMaxHeight(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"zero uses default", 0, defaultScreenshotMaxHeight},
		{"negative uses default", -5, defaultScreenshotMaxHeight},
		{"valid value passes through", 8000, 8000},
		{"oversized value clamps", maxScreenshotMaxHeight + 1, maxScreenshotMaxHeight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampScreenshotMaxHeight(tt.in); got != tt.want {
				t.Errorf("clampScreenshotMaxHeight(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestClampScreenshotHeight(t *testing.T) {
	tests := []struct {
		name          string
		height        int
		max           int
		wantHeight    int
		wantTruncated bool
	}{
		{"short page passes through", 4000, 16384, 4000, false},
		{"page at the cap passes through", 16384, 16384, 16384, false},
		{"tall page clamps and flags", 26000, 16384, 16384, true},
		{"zero cap disables clamping", 26000, 0, 26000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHeight, gotTruncated := clampScreenshotHeight(tt.height, tt.max)
			if gotHeight != tt.wantHeight || gotTruncated != tt.wantTruncated {
				t.Errorf("clampScreenshotHeight(%d, %d) = (%d, %v), want (%d, %v)",
					tt.height, tt.max, gotHeight, gotTruncated, tt.wantHeight, tt.wantTruncated)
			}
		})
	}
}

func TestRequestTrackerTracksInFlightRequests(t *testing.T) {
	tracker := newRequestTracker()
	if tracker.busy() {
		t.Fatal("new tracker should be idle")
	}

	tracker.start("req-1")
	if !tracker.busy() {
		t.Fatal("tracker should be busy while a request is in flight")
	}

	// Redirects re-fire RequestWillBeSent for the same request ID; the
	// tracker must still go idle after the single LoadingFinished.
	tracker.start("req-1")
	tracker.finish("req-1")
	if tracker.busy() {
		t.Error("duplicate starts must not outlive the single finish")
	}

	// A failed request releases its slot too.
	tracker.start("req-2")
	tracker.finish("req-2")
	if tracker.busy() {
		t.Error("failed request should release its slot")
	}
}

func TestResolveScreenshotParams_IgnoresBadFormat(t *testing.T) {
	p := resolveScreenshotParams(&domain.ScreenshotOptions{Format: "bmp"})
	if p.format != "jpeg" {
		t.Errorf("unknown format should fall back to jpeg, got %q", p.format)
	}
}

func TestBuildActionSteps_UnknownType(t *testing.T) {
	if _, err := buildActionSteps(domain.Action{Type: "explode"}); err == nil {
		t.Error("expected error for unknown action type")
	}
}

func TestBuildActionSteps_WaitSelectorRequiresSelector(t *testing.T) {
	if _, err := buildActionSteps(domain.Action{Type: "wait_selector"}); err == nil {
		t.Error("expected error for missing selector")
	}
}

func TestBuildActionSteps_ClickRequiresSelector(t *testing.T) {
	if _, err := buildActionSteps(domain.Action{Type: "click"}); err == nil {
		t.Error("expected error for missing selector")
	}
}

func TestBuildActionSteps_WaitMs(t *testing.T) {
	steps, err := buildActionSteps(domain.Action{Type: "wait_ms", Ms: 250})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

func TestBuildActionSteps_ScrollToBottom(t *testing.T) {
	steps, err := buildActionSteps(domain.Action{Type: "scroll_to_bottom"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

// TestService_RejectsActionsInStaticMode verifies actions force dynamic mode
// and error out when static is explicitly requested.
func TestService_RejectsActionsInStaticMode(t *testing.T) {
	svc := NewService(nil, nil, nil)
	_, err := svc.Scrape(context.Background(), "https://example.com", "static", domain.ScrapeOptions{
		Actions: []domain.Action{{Type: "wait_ms", Ms: 10}},
	})
	if err == nil {
		t.Error("expected error for actions in static mode")
	}
}

// TestService_ActionsForceDynamicMode verifies actions upgrade smart to dynamic.
func TestService_ActionsForceDynamicMode(t *testing.T) {
	colly := &mockScraper{err: fmt.Errorf("should not be called")}
	chromedp := &mockScraper{result: newMockResult("chromedp")}
	svc := NewService(colly, chromedp, nil)

	result, err := svc.Scrape(context.Background(), "https://example.com", "smart", domain.ScrapeOptions{
		Actions: []domain.Action{{Type: "wait_ms", Ms: 10}},
	})
	if err != nil {
		t.Fatalf("scrape failed: %v", err)
	}
	if result.Metadata["engine"] != "chromedp" {
		t.Errorf("actions should force dynamic engine, got %q", result.Metadata["engine"])
	}
}

func TestShouldRecycle(t *testing.T) {
	tests := []struct {
		count, after int
		want         bool
	}{
		{99, 100, false},
		{100, 100, true},
		{101, 100, true},
		{5, 0, false}, // disabled
	}
	for _, tt := range tests {
		if got := shouldRecycle(tt.count, tt.after); got != tt.want {
			t.Errorf("shouldRecycle(%d, %d) = %v, want %v", tt.count, tt.after, got, tt.want)
		}
	}
}

// TestBeginScrape_RecyclesAllocator verifies the allocator factory is
// re-invoked once the scrape counter crosses the recycle threshold.
func TestBeginScrape_RecyclesAllocator(t *testing.T) {
	var calls int32
	factory := func() (context.Context, context.CancelFunc) {
		atomic.AddInt32(&calls, 1)
		return context.WithCancel(context.Background())
	}

	s := &ChromedpScraper{recycleAfter: 2, newAllocator: factory}
	s.allocCtx, s.cancel = factory()

	if got := s.beginScrape(); got == nil {
		t.Fatal("beginScrape returned nil context")
	}
	if got := s.beginScrape(); got == nil {
		t.Fatal("beginScrape returned nil context")
	}
	// Threshold (2) crossed on the 2nd scrape → allocator recreated once.
	if c := atomic.LoadInt32(&calls); c != 2 {
		t.Errorf("expected 2 allocator creations (1 init + 1 recycle), got %d", c)
	}
	if s.scrapeCount != 0 {
		t.Errorf("scrapeCount should reset after recycle, got %d", s.scrapeCount)
	}
	if got := s.beginScrape(); got == nil {
		t.Fatal("beginScrape returned nil context after recycle")
	}
	if c := atomic.LoadInt32(&calls); c != 2 {
		t.Errorf("no further recreation expected below threshold, got %d calls", c)
	}
}
