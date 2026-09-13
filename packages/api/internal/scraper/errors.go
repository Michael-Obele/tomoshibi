package scraper

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// StatusError wraps a scrape failure that carries an HTTP status code so
// callers can distinguish retryable (5xx/network) from permanent (4xx)
// failures.
type StatusError struct {
	StatusCode int
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
