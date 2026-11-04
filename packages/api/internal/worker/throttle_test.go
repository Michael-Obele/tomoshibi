package worker

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestDomainThrottle_WaitSerializesSameHost(t *testing.T) {
	throttle := newDomainThrottle(60*time.Millisecond, time.Second)
	start := time.Now()

	if err := throttle.wait(context.Background(), "example.com"); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Millisecond {
		t.Fatalf("first wait took %v, want immediate", elapsed)
	}

	if err := throttle.wait(context.Background(), "example.com"); err != nil {
		t.Fatalf("second wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("second wait finished after %v, want at least one delay", elapsed)
	}
}

func TestDomainThrottle_BacksOffOnRateLimit(t *testing.T) {
	throttle := newDomainThrottle(100*time.Millisecond, 10*time.Second)

	throttle.observe("example.com", 10*time.Millisecond, http.StatusTooManyRequests, 0)
	if got := throttle.delayFor("example.com"); got != 2*time.Second {
		t.Fatalf("delay after 429 = %v, want 2s", got)
	}
	throttle.observe("example.com", 10*time.Millisecond, http.StatusServiceUnavailable, 0)
	if got := throttle.delayFor("example.com"); got != 4*time.Second {
		t.Fatalf("delay after 503 = %v, want 4s", got)
	}
}

func TestDomainThrottle_HonoursRetryAfter(t *testing.T) {
	throttle := newDomainThrottle(100*time.Millisecond, 30*time.Second)
	throttle.observe("example.com", 10*time.Millisecond, http.StatusTooManyRequests, 3*time.Second)

	if got := throttle.delayFor("example.com"); got != 3*time.Second {
		t.Fatalf("delay after Retry-After = %v, want 3s", got)
	}
}

func TestDomainThrottle_RelaxesWhenFast(t *testing.T) {
	throttle := newDomainThrottle(50*time.Millisecond, 10*time.Second)

	// Raise the delay with push-back signals, then show that a healthy origin
	// pulls it back down toward 2x latency.
	throttle.observe("example.com", 10*time.Millisecond, http.StatusTooManyRequests, 0)
	if got := throttle.delayFor("example.com"); got != 2*time.Second {
		t.Fatalf("delay after 429 = %v, want 2s", got)
	}

	for i := 0; i < 30; i++ {
		throttle.observe("example.com", 20*time.Millisecond, http.StatusOK, 0)
	}
	got := throttle.delayFor("example.com")
	if got >= 2*time.Second {
		t.Fatalf("delay = %v, want it to relax after healthy responses", got)
	}
	if got < 40*time.Millisecond {
		t.Fatalf("delay = %v, want it to stay at or above 2x latency", got)
	}
}

func TestDomainThrottle_ClampsToMax(t *testing.T) {
	throttle := newDomainThrottle(0, 2*time.Second)
	for i := 0; i < 10; i++ {
		throttle.observe("example.com", time.Millisecond, http.StatusTooManyRequests, 0)
	}
	if got := throttle.delayFor("example.com"); got != 2*time.Second {
		t.Fatalf("delay = %v, want the 2s cap", got)
	}
}

func TestDomainThrottle_WaitRespectsContext(t *testing.T) {
	throttle := newDomainThrottle(5*time.Second, 10*time.Second)
	if err := throttle.wait(context.Background(), "example.com"); err != nil {
		t.Fatalf("first wait: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := throttle.wait(ctx, "example.com"); err == nil {
		t.Fatal("expected the reserved wait to be cancelled")
	}
}
