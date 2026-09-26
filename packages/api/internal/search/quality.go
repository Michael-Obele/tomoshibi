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
)

// isWeak reports whether a result set is concentrated enough that the chain
// should keep looking for a better backend.
func isWeak(results []Result) bool {
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
