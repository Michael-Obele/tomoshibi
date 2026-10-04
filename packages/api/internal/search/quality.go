package search

import (
	"net/url"
	"strings"
)

// Weak-set gate for the backend chain (wired into HybridService.Search).
//
// The chain stops at the first non-empty result set, but a backend can answer
// with results that are non-empty yet concentrated on a single domain —
// observed as wikipedia-only answers for topical queries while the other
// general engines are blocked from datacenter egress. Returning those ends the
// chain before SearXNG/Brave ever run, so callers get confidently wrong
// results instead of the fallback's relevant ones.
//
// A concentrated set is therefore kept only as a last resort: the chain keeps
// going, and if no backend returns something stronger the weak set is
// returned rather than nothing.
const (
	// weakMinResults is the smallest set the concentration check considers;
	// below it a single domain is unremarkable (a narrow query can
	// legitimately be answered by one site).
	weakMinResults = 5
	// weakDomainShare is the top-domain share at or above which a result set
	// counts as concentrated (observed: 10/10 wikipedia → off-topic junk).
	weakDomainShare = 0.8
	// thinMinResults is the largest set the thinness check treats as thin. A set
	// with more rows than this carries enough evidence on its own that a
	// single source is no longer suspicious. Three rows is the cut: below it,
	// a same-host set is almost always a blocked upstream answering with a
	// token response rather than a genuinely one-page answer.
	thinMinResults = 4
	// thinDomainShare is the top-domain share at or above which a thin set
	// counts as single-source. A 2-result set is inherently "thin"; the
	// share check is what separates "one site answered this" from "several
	// sites agreed this is a one-page answer".
	thinDomainShare = 0.99
)

// isWeak reports whether a result set is concentrated enough — or too thin —
// that the chain should keep looking for a better backend.
//
// Two distinct failure modes end a search early, and both used to be invisible:
//
//  1. Concentration. Several engines are blocked from datacenter egress, so a
//     set arrives dominated by whichever site still answers (wikipedia-only).
//  2. Thinness. An upstream is blocked and returns a token 1-2 row response.
//     The set is non-empty, so the chain accepted it — even when every row
//     came from one domain and the total is too small to trust.
//
// The second case is why a bare `len(results) < weakMinResults => not weak`
// shortcut is wrong: it is precisely the small, single-source set that most
// needs a second opinion.
//
// opts is consulted because concentration is only a symptom when the caller
// did not ask for it. With includeDomains=["go.dev"], a result set that is
// 10/10 go.dev is the correct answer, not a blocked upstream answering with
// wikipedia — treating it as weak made the chain walk past a perfectly good
// backend and hand the caller a worse one. Only includeDomains pins the shape
// like that; excludeDomains leaves the expected shape unchanged, so the gate
// still applies there.
func isWeak(results []Result, opts SearchOptions) bool {
	if len(results) == 0 {
		return false
	}
	if len(opts.IncludeDomains) > 0 {
		return false
	}
	if isThin(results) {
		return true
	}
	if len(results) < weakMinResults {
		return false
	}
	counts := make(map[string]int, len(results))
	for _, r := range results {
		counts[resultHost(r)]++
	}
	top := 0
	for _, n := range counts {
		if n > top {
			top = n
		}
	}
	return float64(top) >= weakDomainShare*float64(len(results))
}

// isThin reports whether a result set is too small and too single-source to
// be trusted as a complete answer.
//
// A tiny set from several domains is left alone: a genuinely narrow query
// ("sqlite fts5 porter tokenizer") can legitimately be answered by one or two
// pages, and forcing the chain onward would trade a correct answer for a
// slower, possibly worse one. The gate only fires when the small set is also
// concentrated — the signature of a blocked upstream rather than a narrow
// question.
func isThin(results []Result) bool {
	// thinDomainShare is 0.99, so this reduces to "every row shares one
	// host". Comparing explicitly keeps the intent readable and ties the
	// behaviour to the documented constant.
	if len(results) >= thinMinResults {
		return false
	}
	// Fewer than thinMinResults rows: only a single source is unconvincing.
	for _, r := range results {
		if resultHost(r) != resultHost(results[0]) {
			return false
		}
	}
	return float64(len(results)) >= thinDomainShare*float64(len(results))
}

// resultHost is the bucket a result is counted under: Domain when the backend
// filled it, otherwise the URL host — so backends that leave Domain empty
// still participate in the concentration check. www. is stripped because the
// two sources disagree about it.
func resultHost(r Result) string {
	if r.Domain != "" {
		return strings.TrimPrefix(r.Domain, "www.")
	}
	u, err := url.Parse(r.URL)
	if err != nil || u.Host == "" {
		return r.URL // unparsable: its own bucket, never merges with others
	}
	return strings.TrimPrefix(u.Host, "www.")
}
