package scraper

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// StatusError wraps a scrape failure that carries an HTTP status code so
// callers can distinguish retryable (5xx/network) from permanent (4xx)
// failures. RetryAfter is populated from the origin's Retry-After header when
// it sent one, so the crawl throttle can obey it instead of guessing.
type StatusError struct {
	StatusCode int
	RetryAfter time.Duration
	Err        error
}

// Error implements the error interface.
func (e *StatusError) Error() string {
	return fmt.Sprintf("scrape returned status %d: %v", e.StatusCode, e.Err)
}

// Unwrap exposes the wrapped error for errors.As/Is.
func (e *StatusError) Unwrap() error {
	return e.Err
}

// BlockedError reports that the origin answered with an anti-bot challenge
// instead of content. It is deliberately distinct from a generic scrape
// failure: the body is not page content, so returning it with a success status
// would make every downstream consumer reason about a block page as if it were
// the page they asked for.
//
// Callers can surface it to a human, retry through another route, or hand it
// to an optional solver addon — but they must never treat it as data.
type BlockedError struct {
	URL        string
	Vendor     string
	StatusCode int
	// HTML is the challenge page as fetched. It is kept so an optional solver
	// addon can inspect what it is dealing with; it is never returned to a
	// caller as content.
	HTML string
	Err  error
}

// Error implements the error interface.
func (e *BlockedError) Error() string {
	vendor := e.Vendor
	if vendor == "" {
		vendor = "unknown"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("blocked by %s anti-bot challenge (HTTP %d) at %s", vendor, e.StatusCode, e.URL)
	}
	return fmt.Sprintf("blocked by %s anti-bot challenge at %s", vendor, e.URL)
}

// Unwrap exposes the wrapped error for errors.As/Is.
func (e *BlockedError) Unwrap() error {
	return e.Err
}

// RetryAfter returns how long the origin asked us to wait, when the block was
// reported as a status code rather than a 200 interstitial.
func (e *BlockedError) RetryAfter() time.Duration {
	var se *StatusError
	if errors.As(e.Err, &se) {
		return se.RetryAfter
	}
	return 0
}

// worthRetryingDynamic reports whether a failed static scrape is worth
// retrying in a real browser.
//
// The distinction matters because a retry costs a browser tab and several
// seconds. Only two classes of failure plausibly succeed on the second
// attempt: anti-bot responses that key on the absence of a JS runtime, and
// an empty body (frequently a shell whose content is script-injected).
// DNS, TLS, and connection-refused errors fail identically in Chrome.
func worthRetryingDynamic(err error) bool {
	if err == nil {
		return false
	}

	// A challenge page is exactly what a real browser sometimes gets past, so
	// it is always worth the one speculative retry in smart mode.
	var blocked *BlockedError
	if errors.As(err, &blocked) {
		return true
	}

	var se *StatusError
	if errors.As(err, &se) {
		switch se.StatusCode {
		case http.StatusUnauthorized, // 401
			http.StatusForbidden,          // 403 — the common Cloudflare/WAF block
			http.StatusNotAcceptable,      // 406 — some WAFs use this
			http.StatusTooManyRequests,    // 429
			http.StatusServiceUnavailable: // 503 — challenge pages
			return true
		}
		return false
	}

	// colly reports a body-less 200 as this sentinel; it is often an SPA
	// shell that only populates under JS.
	return strings.Contains(err.Error(), "empty response")
}
