package engines

import (
	"net/url"
	"strings"
)

// Site-operator injection.
//
// An engine that understands `site:` can be told which domains the caller
// cares about and will spend its whole result page on them. Without the
// operator we ask for 20 hits, discard 18 in post-filtering and hand back 2:
// the operator is what turns a domain filter from a filter into a query.
//
// Two rules keep this honest:
//
//  1. It is opt-in per engine (Spec.SiteOperator). Engines backed by a plain
//     JSON or RSS API — github, npm, wikipedia, mdn, crates — have no `site:`
//     operator, so the injected token would be matched as a literal search
//     term and every row would be nonsense. Leaving the flag off is always
//     correct, just thinner.
//
//  2. It is a recall optimisation, never the contract. The parent package
//     still post-filters, because no engine applies the operator perfectly
//     and some ignore it outright. Injection can only ever add in-domain
//     candidates; only the post-filter decides what is kept.
//
// Verified upstream support (2026-10): DuckDuckGo, Bing, Brave, Marginalia.
// Unverified and therefore off: mwmbl.

// maxSiteOperators caps how many domain filters are injected into one query.
// Engines truncate long queries, and past a handful the operator soup costs
// more recall than post-filtering ever would.
const maxSiteOperators = 8

// siteOperatorQuery appends `site:` / `-site:` operators to base for engines
// that declare the capability.
//
// base is returned unchanged when it is blank. That guard is not cosmetic:
// Marginalia reads a bare `site:example.com` as "show me site info", not
// "search this site", so injecting into an empty query would turn a search
// into a different request entirely.
func siteOperatorQuery(base string, q Query) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	include := normalizeDomains(q.IncludeDomains)
	exclude := normalizeDomains(q.ExcludeDomains)

	ops := make([]string, 0, len(include)+len(exclude))
	for _, d := range include {
		ops = append(ops, "site:"+d)
	}
	for _, d := range exclude {
		ops = append(ops, "-site:"+d)
	}
	if len(ops) == 0 {
		return base
	}
	return base + " " + strings.Join(ops, " ")
}

// normalizeDomains cleans, dedupes and caps a domain filter list.
func normalizeDomains(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(list))
	out := make([]string, 0, len(list))
	for _, d := range list {
		d = normalizeDomain(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
		if len(out) == maxSiteOperators {
			break
		}
	}
	return out
}

// normalizeDomain reduces a user-supplied domain filter to a bare hostname
// suitable for a `site:` operator.
//
// It is deliberately more forgiving than the parent package's domainSet, which
// only trims a "*." prefix and therefore never matches a caller who passed
// "https://go.dev/blog". Being lenient here is safe because the post-filter
// remains authoritative — this only decides what we ask upstream for.
func normalizeDomain(raw string) string {
	d := strings.ToLower(strings.TrimSpace(raw))
	if d == "" {
		return ""
	}
	// Strip a scheme by parsing rather than splitting on "://", so a bare
	// "example.com:8080/path" and "https://example.com" both land on the
	// hostname.
	if strings.Contains(d, "://") {
		if u, err := url.Parse(d); err == nil && u.Hostname() != "" {
			d = u.Hostname()
		} else {
			d = ""
		}
	}
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	// A wildcard prefix is redundant for `site:` — every engine that supports
	// the operator already treats example.com as covering its subdomains, and
	// "site:*.example.com" is a literal-token miss on DuckDuckGo and Bing.
	// Marginalia accepts the glob form, but the bare form works there too.
	d = strings.TrimPrefix(d, "*.")
	// A stray port is not part of a `site:` operand.
	if i := strings.LastIndex(d, ":"); i > 0 && !strings.Contains(d[i:], "]") {
		d = d[:i]
	}
	d = strings.Trim(d, ".")
	return d
}
