// Package engines implements Tomoshibi's in-house metasearch engine layer:
// a declarative YAML registry of upstream search engines (HTML SERP, RSS,
// JSON API), a shared fetch layer with browser-grade TLS impersonation
// (bogdanfinn/tls-client) and proxy rotation, and per-engine parsers.
//
// Clean-room rule (plan README R4): everything here is built from live
// responses, public API docs, and fixtures — never from SearXNG source (AGPL).
package engines

import (
	"context"
	"errors"
	"time"
)

// Query is the normalized upstream query passed to an engine.
type Query struct {
	// Q is the raw user query.
	Q string
	// Pageno is the 1-based page number (SearXNG semantics).
	Pageno int
	// Language is an ISO code such as "en"; "" means any language.
	Language string
	// TimeRange is "", "day", "week" or "month".
	TimeRange string
}

// Result is one normalized hit from an engine. Engines never import the
// parent search package (that would cycle); native.go converts these.
type Result struct {
	Title       string
	URL         string
	Description string
	Published   *time.Time
	// Engine is the engine name that produced this result.
	Engine string
}

// Sentinel errors surfaced to the fan-out layer. ErrBlocked maps to an
// unresponsive_engines entry with a block reason; ErrNotConfigured means a
// required env var (e.g. an API key) is absent so the engine is skipped
// rather than burned on a guaranteed 401.
var (
	// ErrBlocked marks HTTP-level blocks: 403/429 or a captcha/anomaly
	// marker in an otherwise-200 body.
	ErrBlocked = errors.New("engine blocked")
	// ErrNotConfigured marks engines whose requires_env variable is empty.
	ErrNotConfigured = errors.New("engine not configured")
)

// Engine runs one query against one upstream. Implementations must be safe
// for concurrent use — the fan-out layer calls them in parallel.
type Engine interface {
	// Name matches the registry entry name (and the SearXNG engine string
	// we emit in results and unresponsive_engines).
	Name() string
	// Categories is the subset of {"general","news","it"} this engine serves.
	Categories() []string
	// Search executes q and returns normalized results. A non-nil error
	// marks the engine unresponsive for this query.
	Search(ctx context.Context, q Query) ([]Result, error)
}
