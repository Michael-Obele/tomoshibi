package extract

import (
	"regexp"
	"sort"
	"strings"
)

// sentenceSplit splits text on sentence-ending punctuation or line breaks.
var sentenceSplit = regexp.MustCompile(`(?m)[.!?。！？]\s+|\n\s*\n`)

// wordSplit isolates word tokens for frequency scoring.
var wordSplit = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Summarize returns an extractive summary of markdown. A readability
// excerpt wins when provided; otherwise the top `sentences` sentences by
// term frequency (position-bonused) are returned in document order.
func Summarize(markdown string, excerpt string, sentences int) string {
	if sentences <= 0 {
		sentences = 5
	}
	if excerpt != "" {
		// Strip markdown syntax for a cleaner excerpt.
		return cleanText(excerpt)
	}

	parts := sentenceSplit.Split(markdown, -1)
	if len(parts) == 0 {
		return ""
	}

	// Term frequency across the document.
	freq := make(map[string]int)
	for _, part := range parts {
		for _, tok := range wordSplit.Split(strings.ToLower(part), -1) {
			if len(tok) > 2 {
				freq[tok]++
			}
		}
	}

	type scored struct {
		idx   int
		score float64
	}
	scoredList := make([]scored, 0, len(parts))
	for i, part := range parts {
		clean := cleanText(part)
		if len(clean) < 20 {
			continue // skip fragments
		}
		var score float64
		for _, tok := range wordSplit.Split(strings.ToLower(part), -1) {
			if f, ok := freq[tok]; ok && len(tok) > 2 {
				score += float64(f)
			}
		}
		// Position bonus: sentences near the start are more likely key.
		score *= 1.0 / float64(1+len(scoredList)*2)
		scoredList = append(scoredList, scored{idx: i, score: score})
	}

	sort.SliceStable(scoredList, func(a, b int) bool {
		return scoredList[a].score > scoredList[b].score
	})
	if len(scoredList) > sentences {
		scoredList = scoredList[:sentences]
	}
	// Restore document order.
	sort.Slice(scoredList, func(a, b int) bool {
		return scoredList[a].idx < scoredList[b].idx
	})

	out := make([]string, 0, len(scoredList))
	for _, s := range scoredList {
		if c := cleanText(parts[s.idx]); c != "" {
			out = append(out, c)
		}
	}
	return strings.Join(out, " ")
}

// cleanText strips common markdown syntax and collapses whitespace.
func cleanText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Remove markdown headings, links, emphasis, and code ticks.
	for _, repl := range []struct{ in, out string }{
		{`#`, ""}, {`*`, ""}, {"`", ""}, {"_", ""},
	} {
		s = strings.ReplaceAll(s, repl.in, repl.out)
	}
	s = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`).ReplaceAllString(s, `$1`)
	return strings.Join(strings.Fields(s), " ")
}
