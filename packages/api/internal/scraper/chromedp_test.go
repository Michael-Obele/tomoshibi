package scraper

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
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
	if _, err := buildActionSteps(domain.Action{Type: "explode"}, nil); err == nil {
		t.Error("expected error for unknown action type")
	}
}

func TestBuildActionSteps_WaitSelectorRequiresSelector(t *testing.T) {
	if _, err := buildActionSteps(domain.Action{Type: "wait_selector"}, nil); err == nil {
		t.Error("expected error for missing selector")
	}
}

func TestBuildActionSteps_ClickRequiresSelector(t *testing.T) {
	if _, err := buildActionSteps(domain.Action{Type: "click"}, nil); err == nil {
		t.Error("expected error for missing selector")
	}
}

func TestBuildActionSteps_WaitMs(t *testing.T) {
	steps, err := buildActionSteps(domain.Action{Type: "wait_ms", Ms: 250}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

func TestBuildActionSteps_ScrollToBottom(t *testing.T) {
	steps, err := buildActionSteps(domain.Action{Type: "scroll_to_bottom"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

func TestBuildActionSteps_EvaluateRequiresScript(t *testing.T) {
	if _, err := buildActionSteps(domain.Action{Type: "evaluate"}, nil); err == nil {
		t.Error("expected error for missing script")
	}
}

func TestBuildActionSteps_WaitForFunctionRequiresScript(t *testing.T) {
	if _, err := buildActionSteps(domain.Action{Type: "wait_for_function"}, nil); err == nil {
		t.Error("expected error for missing script")
	}
}

func TestBuildActionSteps_EvaluateReturnsOneStep(t *testing.T) {
	steps, err := buildActionSteps(domain.Action{Type: "evaluate", Script: "() => 1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

func TestBuildActionSteps_WaitForFunctionReturnsOneStep(t *testing.T) {
	steps, err := buildActionSteps(domain.Action{Type: "wait_for_function", Script: "() => true", Ms: 100}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

func TestWrapEvaluateScript(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{"bare expression", "document.title"},
		{"arrow function", "() => window.__DATA__"},
		{"multi-line arrow function", "() => {\n\treturn window.__DATA__;\n}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapEvaluateScript(tt.script)
			if !strings.Contains(got, "("+tt.script+")") {
				t.Errorf("wrapped script should embed the original verbatim; got %q", got)
			}
			if !strings.Contains(got, `typeof v === "function"`) {
				t.Errorf("wrapped script should contain the function-invoke guard; got %q", got)
			}
		})
	}
}

func TestCapEvaluateResult(t *testing.T) {
	t.Run("small JSON value passes through", func(t *testing.T) {
		val := map[string]any{"title": "hi"}
		got, truncated := capEvaluateResult(val)
		if truncated {
			t.Error("small value should not be marked truncated")
		}
		if !reflect.DeepEqual(got, val) {
			t.Errorf("small value should pass through unchanged, got %#v", got)
		}
	})

	t.Run("oversized value is truncated to a rune-safe string preview", func(t *testing.T) {
		long := strings.Repeat("a", maxEvaluateResultBytes+10)
		got, truncated := capEvaluateResult(long)
		if !truncated {
			t.Error("oversized value should be marked truncated")
		}
		s, ok := got.(string)
		if !ok {
			t.Fatalf("truncated result should be a string, got %T", got)
		}
		if len(s) > maxEvaluateResultBytes {
			t.Errorf("preview length %d exceeds cap %d", len(s), maxEvaluateResultBytes)
		}
		if !utf8.ValidString(s) {
			t.Error("preview should be valid UTF-8")
		}
	})
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

func TestResolveScreenshotParams_Scale(t *testing.T) {
	tests := []struct {
		name string
		opts *domain.ScreenshotOptions
		want string
	}{
		{"nil defaults to css", nil, "css"},
		{"empty defaults to css", &domain.ScreenshotOptions{}, "css"},
		{"css passes through", &domain.ScreenshotOptions{Scale: "css"}, "css"},
		{"device opt-in passes through", &domain.ScreenshotOptions{Scale: "device"}, "device"},
		{"unknown value falls back to css", &domain.ScreenshotOptions{Scale: "retina"}, "css"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveScreenshotParams(tt.opts).scale; got != tt.want {
				t.Errorf("scale = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestViewportActionScale pins the capture emulation each scale selects: the
// default "css" branch keeps the historical 1x viewport, while "device" opts
// into 2x device pixels. Both branches return chromedp.Tasks whose first
// action is a *emulation.SetDeviceMetricsOverrideParams; the asserted field is
// DeviceScaleFactor, which is what carries the 1x/2x choice (the type's
// unrelated Scale field is left at its zero value by both branches).
func TestViewportActionScale(t *testing.T) {
	tests := []struct {
		scale     string
		wantScale float64
	}{
		{"css", 1.0},
		{"device", 2.0},
	}
	for _, tc := range tests {
		t.Run(tc.scale, func(t *testing.T) {
			tasks, ok := viewportAction(screenshotParams{width: 800, height: 600, scale: tc.scale}).(chromedp.Tasks)
			if !ok {
				t.Fatalf("viewportAction(%q) is not chromedp.Tasks", tc.scale)
			}
			first, ok := tasks[0].(*emulation.SetDeviceMetricsOverrideParams)
			if !ok {
				t.Fatalf("first task = %T, want *emulation.SetDeviceMetricsOverrideParams", tasks[0])
			}
			if first.DeviceScaleFactor != tc.wantScale {
				t.Errorf("DeviceScaleFactor = %v, want %v", first.DeviceScaleFactor, tc.wantScale)
			}
		})
	}
}

func TestDocumentStatusError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   int // 0 means "no error"
	}{
		{"ok is not an error", 200, 0},
		{"redirect is not an error", 302, 0},
		{"last non-error status", 399, 0},
		{"bad request surfaces", 400, 400},
		{"forbidden surfaces", 403, 403},
		{"not found surfaces", 404, 404},
		{"server error surfaces", 500, 500},
		{"unobserved status is not an error", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := documentStatusError(tt.status)
			if tt.want == 0 {
				if err != nil {
					t.Fatalf("status %d: got error %v, want nil", tt.status, err)
				}
				return
			}
			var se *StatusError
			if !errors.As(err, &se) {
				t.Fatalf("status %d: error %v is not a *StatusError", tt.status, err)
			}
			if se.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", se.StatusCode, tt.want)
			}
		})
	}
}

func TestDocumentStatus_MainFrameOnly(t *testing.T) {
	tests := []struct {
		name   string
		events []struct {
			frame cdp.FrameID
			code  int
		}
		want int
	}{
		{
			name: "iframe error does not fail a healthy page",
			events: []struct {
				frame cdp.FrameID
				code  int
			}{{"MAIN", 200}, {"IFRAME", 404}},
			want: 200,
		},
		{
			name: "iframe 200 does not mask a main-frame 404",
			events: []struct {
				frame cdp.FrameID
				code  int
			}{{"MAIN", 404}, {"IFRAME", 200}},
			want: 404,
		},
		{
			name:   "no events leaves the status unobserved",
			events: nil,
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &documentStatus{}
			for _, ev := range tt.events {
				d.record(ev.frame, ev.code)
			}
			if got := d.get(); got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
		})
	}
}
