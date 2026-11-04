package scraper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Network capture limits. These are deliberately smaller than Scrapling's
// published ceiling (1,000 responses / 1 MiB per body / 20 MiB total) because
// the target deployment is a 512 MB host: a page with a few large API
// responses must not be able to push the process into swap.
const (
	captureMaxResponses  = 100
	captureMaxBodyBytes  = 512 << 10
	captureMaxTotalBytes = 8 << 20
	captureDrainTimeout  = 2 * time.Second
	captureIdleTimeout   = 3 * time.Second

	// Chromium keeps response bodies in the renderer. Its desktop default is
	// 200 MB total / 20 MB per resource, which is far beyond this service's
	// budget. The Go caps above are the real limit; these are set slightly
	// higher so Chromium does not evict a body before it is fetched.
	captureChromeTotalBuffer    = 16 << 20
	captureChromeResourceBuffer = 1 << 20
)

// capturedEntry is what the Network events tell us about one request before
// its body is fetched. Redirects reuse a RequestID, so later events overwrite
// earlier ones and the final hop is what gets recorded.
type capturedEntry struct {
	url          string
	method       string
	status       int
	contentType  string
	resourceType string
	sizeBytes    int
	fromCache    bool
	redirected   bool
	eligible     bool
	truncated    bool
	bodyError    string
}

type captureJob struct {
	id    network.RequestID
	entry capturedEntry
}

// networkCollector listens to CDP Network events for one tab and records the
// XHR/fetch responses that look like data.
//
// One listener, one body-fetch goroutine. Fetching a body inside the listener
// would deadlock the target loop (listeners run synchronously), and one
// goroutine per response would race eviction and explode on request-heavy
// pages.
type networkCollector struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	entries   map[network.RequestID]capturedEntry
	responses []domain.CapturedResponse
	seen      map[string]int
	accepted  int
	total     int
	dropped   int
	truncated bool

	bodies     chan captureJob
	done       chan struct{}
	bodiesDone chan struct{}
}

// newNetworkCollector attaches the listener to the tab context. It must be
// called before navigation so no requests are missed.
func newNetworkCollector(ctx context.Context) *networkCollector {
	fetchCtx, cancel := context.WithCancel(ctx)
	c := &networkCollector{
		ctx:        fetchCtx,
		cancel:     cancel,
		entries:    make(map[network.RequestID]capturedEntry),
		seen:       make(map[string]int),
		bodies:     make(chan captureJob, captureMaxResponses),
		done:       make(chan struct{}),
		bodiesDone: make(chan struct{}),
	}
	chromedp.ListenTarget(ctx, c.handleEvent)
	go c.collectBodies()
	return c
}

// handleEvent runs on the target loop. It must only update state and enqueue;
// any blocking work here stalls every other CDP event.
func (c *networkCollector) handleEvent(ev any) {
	switch e := ev.(type) {
	case *network.EventRequestWillBeSent:
		c.onRequest(e)
	case *network.EventResponseReceived:
		c.onResponse(e)
	case *network.EventLoadingFinished:
		c.onFinished(e)
	case *network.EventLoadingFailed:
		c.mu.Lock()
		delete(c.entries, e.RequestID)
		c.mu.Unlock()
	}
}

func (c *networkCollector) onRequest(e *network.EventRequestWillBeSent) {
	if e.Request == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[e.RequestID]
	entry.url = e.Request.URL
	entry.method = e.Request.Method
	if e.Type != "" {
		entry.resourceType = string(e.Type)
	}
	if e.RedirectResponse != nil {
		entry.redirected = true
	}
	c.entries[e.RequestID] = entry
}

func (c *networkCollector) onResponse(e *network.EventResponseReceived) {
	if e.Response == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[e.RequestID]
	entry.url = e.Response.URL
	entry.status = int(e.Response.Status)
	entry.contentType = e.Response.MimeType
	entry.fromCache = e.Response.FromDiskCache ||
		e.Response.FromPrefetchCache ||
		e.Response.FromServiceWorker
	entry.sizeBytes = int(e.Response.EncodedDataLength)
	if e.Type != "" {
		entry.resourceType = string(e.Type)
	}
	entry.eligible = capturableType(e.Type, e.Response.MimeType, entry.status)
	c.entries[e.RequestID] = entry
}

func (c *networkCollector) onFinished(e *network.EventLoadingFinished) {
	c.mu.Lock()
	entry, ok := c.entries[e.RequestID]
	delete(c.entries, e.RequestID)
	if !ok || !entry.eligible {
		c.mu.Unlock()
		return
	}
	if entry.sizeBytes == 0 {
		entry.sizeBytes = int(e.EncodedDataLength)
	}
	if c.accepted >= captureMaxResponses {
		c.dropped++
		c.mu.Unlock()
		return
	}
	c.accepted++

	// Decide before fetching: an oversized or over-budget body is never
	// copied into the Go heap at all.
	if entry.sizeBytes > captureMaxBodyBytes {
		entry.truncated = true
		entry.bodyError = "body exceeds 512 KiB"
		c.mu.Unlock()
		c.record(entry, "")
		return
	}
	if c.total >= captureMaxTotalBytes {
		entry.truncated = true
		entry.bodyError = "capture budget exhausted"
		c.mu.Unlock()
		c.record(entry, "")
		return
	}
	c.mu.Unlock()

	select {
	case c.bodies <- captureJob{id: e.RequestID, entry: entry}:
	default:
		entry.bodyError = "capture queue full"
		c.record(entry, "")
	}
}

// collectBodies is the single consumer of the body-fetch queue. It drains
// whatever is queued when finish closes done, then exits.
func (c *networkCollector) collectBodies() {
	defer close(c.bodiesDone)
	for {
		select {
		case job := <-c.bodies:
			c.fetchBody(job)
		case <-c.done:
			for {
				select {
				case job := <-c.bodies:
					c.fetchBody(job)
				default:
					return
				}
			}
		}
	}
}

func (c *networkCollector) fetchBody(job captureJob) {
	execCtx := cdp.WithExecutor(c.ctx, chromedp.FromContext(c.ctx).Target)
	body, err := network.GetResponseBody(job.id).Do(execCtx)
	if err != nil {
		job.entry.bodyError = captureError(err)
		c.record(job.entry, "")
		return
	}
	// Do decodes CDP's base64 for us. The MIME allowlist keeps binary out; if
	// a text-labelled response still decodes to invalid UTF-8, it is not
	// useful as text and is omitted rather than mangled.
	if !utf8.Valid(body) {
		job.entry.bodyError = "non-text body omitted"
		c.record(job.entry, "")
		return
	}
	if len(body) > captureMaxBodyBytes {
		job.entry.truncated = true
		job.entry.bodyError = "body exceeds 512 KiB"
		c.record(job.entry, "")
		return
	}
	c.record(job.entry, string(body))
}

// record appends one response, collapsing byte-identical repeats into a count.
// It is the only writer of responses and seen, and it takes the lock itself so
// both the event path and the body path can call it.
func (c *networkCollector) record(entry capturedEntry, body string) {
	entry.url = redactCapturedURL(entry.url)

	c.mu.Lock()
	defer c.mu.Unlock()

	if body != "" && c.total+len(body) > captureMaxTotalBytes {
		entry.truncated = true
		entry.bodyError = "capture budget exhausted"
		body = ""
	}
	if entry.truncated {
		c.truncated = true
	}

	if body != "" {
		key := captureHash(entry, body)
		if idx, ok := c.seen[key]; ok {
			c.responses[idx].Count++
			return
		}
		c.seen[key] = len(c.responses)
	}

	size := len(body)
	if size == 0 {
		size = entry.sizeBytes
	}

	c.responses = append(c.responses, domain.CapturedResponse{
		URL:          entry.url,
		Method:       entry.method,
		Status:       entry.status,
		ContentType:  entry.contentType,
		ResourceType: entry.resourceType,
		Body:         body,
		SizeBytes:    size,
		Truncated:    entry.truncated,
		FromCache:    entry.fromCache,
		Redirected:   entry.redirected,
		Count:        1,
		BodyError:    entry.bodyError,
	})
	if body != "" {
		c.total += len(body)
	}
}

// finish stops accepting work, drains the queue (bounded), and returns a
// snapshot. It returns nil if the drain cannot be completed safely: a partial
// capture beats a data race.
func (c *networkCollector) finish(ctx context.Context) *domain.NetworkCapture {
	close(c.done)

	select {
	case <-c.bodiesDone:
	case <-time.After(captureDrainTimeout):
		c.cancel()
		if !waitOrTimeout(c.bodiesDone, time.Second) {
			return nil
		}
	case <-ctx.Done():
		c.cancel()
		if !waitOrTimeout(c.bodiesDone, time.Second) {
			return nil
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return &domain.NetworkCapture{
		Responses: append([]domain.CapturedResponse(nil), c.responses...),
		Truncated: c.truncated,
		Dropped:   c.dropped,
	}
}

func waitOrTimeout(ch <-chan struct{}, d time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(d):
		return false
	}
}

// capturableType keeps capture to data-bearing XHR/fetch traffic. The document
// is already markdown, images have their own pipeline, and scripts/fonts are
// noise that would only cost memory.
func capturableType(t network.ResourceType, mime string, status int) bool {
	if t != network.ResourceTypeXHR && t != network.ResourceTypeFetch {
		return false
	}
	switch status {
	case 204, 205, 304:
		return false
	}
	return capturableMIME(mime)
}

func capturableMIME(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch m {
	case "application/json",
		"application/x-ndjson",
		"application/xml",
		"text/xml",
		"text/plain",
		"text/csv",
		"application/x-www-form-urlencoded":
		return true
	}
	return strings.HasSuffix(m, "+json") || strings.HasSuffix(m, "+xml")
}

// sensitiveQueryParams are redacted from the recorded URL. Captured API URLs
// routinely carry credentials in the query string, and the body is already
// untrusted; the URL should not add a second leak.
var sensitiveQueryParams = map[string]struct{}{
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"api_key":       {},
	"apikey":        {},
	"key":           {},
	"secret":        {},
	"client_secret": {},
	"signature":     {},
	"sig":           {},
	"password":      {},
	"passwd":        {},
	"session":       {},
	"session_id":    {},
	"sessionid":     {},
	"auth":          {},
	"authorization": {},
	"jwt":           {},
}

func redactCapturedURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := u.Query()
	changed := false
	for name := range query {
		if _, ok := sensitiveQueryParams[strings.ToLower(name)]; ok {
			query.Set(name, "[redacted]")
			changed = true
		}
	}
	if !changed {
		return raw
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func captureHash(entry capturedEntry, body string) string {
	sum := sha256.Sum256([]byte(entry.method + "\n" + entry.url + "\n" + strconv.Itoa(entry.status) + "\n" + body))
	return hex.EncodeToString(sum[:])
}

// captureError keeps CDP's error text short enough for a response field.
func captureError(err error) string {
	const maxLen = 200
	msg := err.Error()
	if len(msg) > maxLen {
		msg = msg[:maxLen]
	}
	return msg
}
