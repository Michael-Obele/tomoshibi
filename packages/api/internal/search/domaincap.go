package search

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"

	"github.com/Michael-Obele/tomoshibi/internal/search/engines"
)

// Per-domain capping.
//
// RRF rewards agreement between engines, and it weights engines by roster
// position. Neither of those stops ONE engine filling an entire page: a
// high-weighted source outranks every other engine's best hit, so its ranks
// 1..10 occupy the top 10. Measured against live engine output, that held 8
// of 10 slots on a shopping query.
//
// The fix is the one the major engines already use — a SERP carries at most
// two results from any one domain. The cap is applied per ENGINE, before
// fusion, so it widens the candidate pool rather than reordering it: the
// discarded slots get filled by the next-best documents from other engines.

// registrableDomain returns the "example.com" of a URL — the registrable
// portion under the Public Suffix List.
//
// It must be the real suffix list, not "the last two labels": under that
// shortcut bbc.co.uk and guardian.co.uk both reduce to "co.uk" and would be
// capped as one source, which would gut the news category. publicsuffix also
// collapses language subdomains correctly (en.wikipedia.org → wikipedia.org,
// de.wikipedia.org → wikipedia.org) while keeping distinct sites apart
// (rust-lang.github.io → github.io, so it shares a bucket with other
// user/org pages on that host — which is the behaviour we want).
func registrableDomain(rawURL string) string {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		// Bucket unparseable URLs by their own literal text. Returning ""
		// here would pool every malformed result into one bucket and let the
		// cap silently delete all but N of them.
		return strings.ToLower(raw)
	}
	host := strings.ToLower(u.Hostname())
	// An IP literal has no registrable form; bucket it by itself.
	if net.ParseIP(host) != nil {
		return host
	}
	if host == "" {
		return raw
	}
	d, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		// Unparseable against the list (e.g. a bare internal hostname).
		// Fall back to the host rather than collapsing everything to "".
		return host
	}
	return d
}

// capPerDomain returns at most max results from any one registrable domain,
// preserving the engine's original order. max <= 0 disables capping.
//
// Order matters: the engine's own ranking is the best signal of which of ITS
// own results matter most, so the first `max` for a domain are the ones kept.
func capPerDomain(in []engines.Result, max int) []engines.Result {
	if max <= 0 || len(in) == 0 {
		return in
	}
	seen := make(map[string]int, len(in))
	out := make([]engines.Result, 0, len(in))
	for _, r := range in {
		d := registrableDomain(r.URL)
		if seen[d] >= max {
			continue
		}
		seen[d]++
		out = append(out, r)
	}
	return out
}
