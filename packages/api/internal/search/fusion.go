package search

import (
	"sort"

	"github.com/Michael-Obele/tomoshibi/internal/search/engines"
)

// rrfK is the RRF damping constant from Cormack, Clarke & Buettcher (2009).
//
// 60 is the value in the paper and the value every production deployment
// converges on. It bounds how much a single engine's opinion can contribute:
// the first position is worth 1/(k+1), the second 1/(k+2), so with k=60 a
// #1 is worth ~1.7x a #2, and the curve flattens fast. That flattening is the
// point — it stops one engine shouting "this is my #1" from overwhelming six
// engines quietly agreeing the result is good.
const rrfK = 60

// rrfScore is the contribution of one engine ranking a document at the given
// 1-based rank.
//
// Rank is 1-based because that is how upstream engines number their results;
// rank <= 0 is treated as 1 so a malformed position can never divide by ~60
// and dominate the total.
func rrfScore(rank int) float64 {
	if rank < 1 {
		rank = 1
	}
	return 1.0 / float64(rrfK+rank)
}

// fusion is the working state for one reciprocal-rank-fusion pass.
type fusion struct {
	// score is the accumulated RRF total per canonical URL.
	score map[string]float64
	// ranks counts how many engines surfaced each document. A URL found by
	// many engines is corroborated; a URL found once may be an outlier.
	ranks map[string]int
	// best holds the highest 1-based rank any engine gave the document.
	best map[string]int
	// weight holds the summed engine weight behind each document, used as a
	// late tiebreaker so a roster that trusts one engine can express it
	// without letting that engine dominate.
	weight map[string]float64
}

func newFusion() *fusion {
	return &fusion{
		score:  make(map[string]float64),
		ranks:  make(map[string]int),
		best:   make(map[string]int),
		weight: make(map[string]float64),
	}
}

// add records one engine's opinion about a document.
//
// rank is the document's 1-based position within that engine's own results.
// weight is the engine's configured weight (higher = more trusted) and scales
// that engine's contribution, so a curated roster can express "trust Brave
// more than Marginalia" without comparing absolute upstream scores.
func (f *fusion) add(key string, rank int, weight float64) {
	if key == "" {
		return
	}
	if weight <= 0 {
		weight = 1
	}
	f.score[key] += rrfScore(rank) * weight
	f.ranks[key]++
	f.weight[key] += weight
	if rank < f.best[key] {
		f.best[key] = rank
	}
}

// fuseEngine is one engine's contribution to a fusion pass.
type fuseEngine struct {
	name    string
	weight  float64
	results []engines.Result
}

// fuse merges engine result lists into a single ranked slice using RRF.
//
// Every engine contributes, and a document found by several engines outranks
// one found by a single engine. The winning entry for each canonical URL is
// the copy from the engine that ranked it best, which preserves the richest
// title/description/highlight rather than whichever engine happened to be
// merged first.
//
// The returned slice is sorted by fused score descending, with deterministic
// tiebreakers (best rank, then corroboration, then summed weight, then
// first-seen order) so identical inputs always produce identical output.
func fuse(engines []fuseEngine) []Result {
	f := newFusion()
	// firstCopy and bestRank track which result object to surface per URL.
	firstCopy := make(map[string]Result)
	bestRank := make(map[string]int)
	firstSeen := make(map[string]int)
	var order []string

	for _, e := range engines {
		for pos, r := range e.results {
			key := canonicalURL(r.URL)
			if key == "" {
				continue
			}
			// Promote the engine-level DTO to the public result shape at the
			// boundary, so fusion does not care which engine produced it.
			pub := Result{
				Title:       r.Title,
				URL:         r.URL,
				Description: r.Description,
				Domain:      extractDomain(r.URL),
				Engine:      r.Engine,
				PublishedAt: r.Published,
			}
			if pub.Engine == "" {
				pub.Engine = e.name
			}
			if _, ok := f.score[key]; !ok {
				firstCopy[key] = pub
				bestRank[key] = pos + 1
				firstSeen[key] = len(order)
				order = append(order, key)
			}
			f.add(key, pos+1, e.weight)
			// Keep the copy from whichever engine ranked it highest.
			if pos+1 < bestRank[key] {
				bestRank[key] = pos + 1
				firstCopy[key] = pub
			}
		}
	}

	if len(order) == 0 {
		return nil
	}

	ranked := make([]string, len(order))
	copy(ranked, order)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if f.score[a] != f.score[b] {
			return f.score[a] > f.score[b]
		}
		if f.best[a] != f.best[b] {
			return f.best[a] < f.best[b]
		}
		if f.ranks[a] != f.ranks[b] {
			return f.ranks[a] > f.ranks[b]
		}
		if f.weight[a] != f.weight[b] {
			return f.weight[a] > f.weight[b]
		}
		return firstSeen[a] < firstSeen[b]
	})

	out := make([]Result, 0, len(ranked))
	for _, key := range ranked {
		r := firstCopy[key]
		// Relevance is now the fused score, normalised to 0..1 so existing
		// clients keep seeing a familiar range. The top hit maps to 1.0 and
		// the rest scale relative to it.
		out = append(out, r)
	}

	normaliseRelevance(out, f, ranked)
	return out
}

// normaliseRelevance rescales fused scores onto 0..1, preserving order.
//
// An empty result set, or a set whose top score is zero, is left untouched
// rather than producing NaN.
func normaliseRelevance(out []Result, f *fusion, ranked []string) {
	if len(out) == 0 {
		return
	}
	top := f.score[ranked[0]]
	if top <= 0 {
		return
	}
	for i := range out {
		key := canonicalURL(out[i].URL)
		out[i].Relevance = f.score[key] / top
	}
}
