package worker

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// domainThrottle spaces requests per host and adapts that spacing to what the
// origin is telling us.
//
// A fixed delay is wrong in both directions: it crawls a fast site far slower
// than necessary and a rate-limiting site too fast. This follows the same idea
// as Scrapling's AutoThrottle: track response latency, ease the delay toward a
// multiple of it, and back off hard when the origin answers 429/503 or sends
// Retry-After.
type domainThrottle struct {
	mu    sync.Mutex
	min   time.Duration
	max   time.Duration
	hosts map[string]*hostThrottle
}

type hostThrottle struct {
	// delay is the current spacing between requests to this host.
	delay time.Duration
	// next is the earliest wall-clock time the next request may start. It is
	// a reservation, so concurrent workers queue behind each other instead of
	// all waking at once.
	next time.Time
	// latency is an EWMA of successful response times.
	latency time.Duration
}

func newDomainThrottle(min, max time.Duration) *domainThrottle {
	if min < 0 {
		min = 0
	}
	if max < min {
		max = min
	}
	return &domainThrottle{
		min:   min,
		max:   max,
		hosts: make(map[string]*hostThrottle),
	}
}

// wait blocks until this worker's reserved slot for host. The reservation is
// made before sleeping, so N workers serialize at the current spacing.
func (t *domainThrottle) wait(ctx context.Context, host string) error {
	t.mu.Lock()
	h := t.hostLocked(host)
	now := time.Now()
	start := h.next
	if start.Before(now) {
		start = now
	}
	h.next = start.Add(h.delay)
	t.mu.Unlock()

	wait := time.Until(start)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// observe feeds one completed attempt back into the host's spacing. status is
// the upstream status (200 for a successful scrape); retryAfter is the value
// the origin asked for, when it sent one.
func (t *domainThrottle) observe(host string, latency time.Duration, status int, retryAfter time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	h := t.hostLocked(host)

	switch {
	case retryAfter > 0:
		// The origin told us exactly how long to wait. Take it, and never
		// shorten the existing delay on the same signal.
		h.delay = t.clamp(maxDuration(t.backoffBase(h), retryAfter))
	case status == http.StatusTooManyRequests,
		status == http.StatusServiceUnavailable,
		status == http.StatusForbidden:
		h.delay = t.clamp(t.backoffBase(h) * 2)
	case status >= 200 && status < 400 && latency > 0:
		// An operator who set the floor to zero opted out of politeness
		// spacing; only real push-back should slow them down. Otherwise the
		// latency estimate would silently reintroduce a delay they disabled.
		if t.min == 0 {
			return
		}
		if h.latency == 0 {
			h.latency = latency
		} else {
			h.latency = (h.latency*7 + latency*3) / 10
		}
		target := t.clamp(2 * h.latency)
		// Ease toward the target rather than snapping: one slow response
		// should not double the wall time of an otherwise healthy crawl.
		// The target is clamped to the floor, so this can only relax a delay
		// that was raised by push-back — never undercut the operator's floor.
		if target > h.delay {
			h.delay = t.clamp((h.delay + target) / 2)
		} else if target < h.delay {
			h.delay = t.clamp((h.delay*3 + target) / 4)
		}
	}
}

// delayFor reports the current spacing for host. It exists for tests and logs.
func (t *domainThrottle) delayFor(host string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.hostLocked(host).delay
}

func (t *domainThrottle) hostLocked(host string) *hostThrottle {
	h, ok := t.hosts[host]
	if !ok {
		h = &hostThrottle{delay: t.min}
		t.hosts[host] = h
	}
	return h
}

// backoffBase is the delay to double on a push-back signal. When the
// configured floor is zero (a caller that opted out of politeness), doubling
// zero would never slow down, so a one-second base keeps the backoff real.
func (t *domainThrottle) backoffBase(h *hostThrottle) time.Duration {
	if h.delay < time.Second {
		return time.Second
	}
	return h.delay
}

func (t *domainThrottle) clamp(d time.Duration) time.Duration {
	if d < t.min {
		return t.min
	}
	if d > t.max {
		return t.max
	}
	return d
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
