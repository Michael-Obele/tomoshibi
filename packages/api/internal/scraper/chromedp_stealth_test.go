package scraper

import (
	"context"
	"errors"
	"testing"
)

func TestBuildAllocatorHasStealthFlags(t *testing.T) {
	s := NewChromedpScraperWithLimit(100)
	if s == nil {
		t.Fatal("expected scraper, got nil")
	}
	if !stealthFlagsContain("disable-blink-features") {
		t.Error("expected disable-blink-features=AutomationControlled in allocator opts")
	}
	if !stealthFlagsContain("headless") {
		t.Error("expected headless flag")
	}
	if !stealthFlagsContain("disable-dev-shm-usage") {
		t.Error("expected disable-dev-shm-usage flag")
	}
}

func TestFetchHTMLRespectsContextCancellation(t *testing.T) {
	s := NewChromedpScraperWithLimit(100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.FetchHTML(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context cancellation error, got %v", err)
	}
}
