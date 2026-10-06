// Package envctx carries per-request environment overrides through
// context.Context, so a client (typically the MCP server) can supply search
// API keys on the wire instead of requiring them in the backend's own .env.
//
// The flow: MCP reads keys from its env → sends them as a JSON header
// (X-Tomoshi-Env) on every backend call → middleware validates and attaches
// them to the request context → engine requests resolve ${VAR} placeholders
// and requires_env gates through envctx.Get for THIS request only.
//
// Design constraints:
//   - Overrides are per-request, never process-global: one client's key can
//     never leak into another client's search (unlike os.Setenv).
//   - Allowlist-only: only search-provider API key names are accepted, and
//     values are used solely in outbound engine requests to fixed registry
//     URLs. Infra credentials (Redis, DB, proxy) can never be supplied or
//     overridden this way.
package envctx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Header is the request header carrying per-request env overrides as a JSON
// object of NAME → value, e.g. {"BRAVE_SEARCH_API_KEY":"BSB..."}.
const Header = "X-Tomoshi-Env"

// Limits bounding a supplied header. Keys are short tokens; anything larger
// is malformed or abusive and the whole header is rejected.
const (
	maxHeaderBytes = 8 << 10 // 8 KiB (well under common 16 KiB header caps)
	maxEntries     = 16
	maxValueLen    = 512
)

// namePattern accepts conventional env var names. Belt-and-braces alongside
// the allowlist: even an allowlist mistake cannot smuggle header syntax.
var namePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

// Allowed lists the env var names clients may supply per request.
//
// Policy: search-provider API keys only — values go into outbound engine
// requests (header/URL/body of a fixed registry URL) and nothing else.
// NEVER add infra credentials here (REDIS_URL, WEBSHARE_API_KEY, DB
// passwords): those are operator-owned and read from the process env only.
//
// Add a name in the same change that lands the keyed engine consuming it
// (registry.default.yaml + a fixture test — plan/tomoshi-search/research.md
// §3b carries the contract evidence for each).
func Allowed() []string {
	return []string{
		"BRAVE_SEARCH_API_KEY", // native roster `brave` + legacy BraveService
		"SERPER_API_KEY",       // native roster `serper` (Google SERP API)
		"TAVILY_API_KEY",       // native roster `tavily`
	}
}

// Overrides are validated env values scoped to one request.
type Overrides map[string]string

type ctxKey struct{}

// With returns a context carrying the given overrides. Callers pass already
// validated values (see Parse).
func With(parent context.Context, ov Overrides) context.Context {
	if len(ov) == 0 {
		return parent
	}
	return context.WithValue(parent, ctxKey{}, ov)
}

// Get resolves name from the request's overrides first, then the process
// env. It is the per-request replacement for os.Getenv on the search path.
func Get(ctx context.Context, name string) string {
	if ctx != nil {
		if ov, ok := ctx.Value(ctxKey{}).(Overrides); ok {
			if v, ok := ov[name]; ok {
				return v
			}
		}
	}
	return os.Getenv(name)
}

// Expand expands ${NAME} references like os.ExpandEnv, resolving each name
// through Get so request overrides win over the process env.
func Expand(ctx context.Context, s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return os.Expand(s, func(name string) string { return Get(ctx, name) })
}

// Parse decodes a raw X-Tomoshi-Env header value.
//
// Accepts a JSON object of string values. Names not in Allowed are dropped
// silently (forward-compat: an older backend must not reject a newer
// client), while structurally broken input is an error so misconfiguration
// is visible in logs. Errors never echo values.
func Parse(raw string) (Overrides, error) {
	if len(raw) > maxHeaderBytes {
		return nil, fmt.Errorf("header exceeds %d bytes", maxHeaderBytes)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("not a JSON object of string values: %w", err)
	}
	if len(m) > maxEntries {
		return nil, fmt.Errorf("more than %d entries", maxEntries)
	}

	allowed := make(map[string]struct{})
	for _, name := range Allowed() {
		allowed[name] = struct{}{}
	}

	ov := make(Overrides, len(m))
	for name, value := range m {
		if _, ok := allowed[name]; !ok {
			continue // unknown name: drop, never reject
		}
		if !namePattern.MatchString(name) {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > maxValueLen {
			return nil, fmt.Errorf("value for %s exceeds %d bytes", name, maxValueLen)
		}
		ov[name] = value
	}
	return ov, nil
}
