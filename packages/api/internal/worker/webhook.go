package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
)

// SignatureHeader is the header carrying the HMAC-SHA256 webhook signature.
const SignatureHeader = "X-Tomoshibi-Signature"

// webhookAttempts is the number of delivery attempts before giving up.
const webhookAttempts = 3

// retryBackoffUnit is the base interval for the linear retry backoff used by
// Deliver and scrapeWithRetry: attempt N waits N × this. It is a variable
// rather than a constant purely so tests can collapse the wait — the retry
// tests assert attempt counts, not wall-clock timing, and at the production
// value the sleeps alone accounted for most of this package's test runtime.
// Not an env knob: operators have no reason to tune it, and the crawl-facing
// timings that they do tune already live in crawl_handler.go.
var retryBackoffUnit = time.Second

// webhookTimeout returns the per-attempt HTTP timeout
// (env WEBHOOK_TIMEOUT seconds, default 10s).
func webhookTimeout() time.Duration {
	raw := os.Getenv("WEBHOOK_TIMEOUT")
	if raw == "" {
		return 10 * time.Second
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 10 * time.Second
	}
	return time.Duration(n) * time.Second
}

// Deliver posts payload to webhookURL with an HMAC-SHA256 signature header,
// retrying transient failures up to webhookAttempts times with backoff.
// Delivery failure never fails the crawl — callers treat the error as a
// recorded warning.
//
// The client refuses non-public destinations: webhook_url is caller-supplied
// and would otherwise let anyone POST crawl output to an internal service.
func Deliver(ctx context.Context, webhookURL, secret string, payload []byte) error {
	client := safeurl.Client(webhookTimeout())
	var lastErr error

	for attempt := 1; attempt <= webhookAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(time.Duration(attempt) * retryBackoffUnit):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("webhook request build failed: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(SignatureHeader, "sha256="+signPayload(secret, payload))

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook delivery attempt %d failed: %w", attempt, err)
			// A refused destination is permanent: the address will not become
			// public on attempt 2. Same reasoning as the 4xx case below.
			var blocked *safeurl.ErrBlocked
			var scheme *safeurl.ErrScheme
			if errors.As(err, &blocked) || errors.As(err, &scheme) {
				return lastErr
			}
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("webhook delivery attempt %d returned status %d", attempt, resp.StatusCode)
		// 4xx means the endpoint rejected the payload — retrying won't help.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}
	return lastErr
}

// signPayload returns the hex HMAC-SHA256 of payload keyed by secret.
func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
