package scraper

import (
	"encoding/json"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// Harvest caps (design: docs/plans/2026-10-09-js-rendered-extraction-design.md).
const (
	maxHarvestSources   = 8
	maxHarvestSourceLen = 128 << 10 // 128 KB per source
	maxHarvestTotalLen  = 512 << 10 // 512 KB across all sources
)

// harvestScript collects embedded payload candidates from the rendered page
// and returns them as a JSON string. Presence-gated by construction: a page
// with no candidates yields all-null fields, parseHarvest drops it, and the
// response field stays omitted.
const harvestScript = `(() => {
  const out = { next_data: null, nuxt: null, initial_state: null, flight: null, json_ld: [], canvases: 0 };
  out.canvases = document.querySelectorAll("canvas").length;
  const nd = document.getElementById("__NEXT_DATA__");
  if (nd && nd.textContent) out.next_data = nd.textContent;
  try { if (window.__NUXT__) out.nuxt = JSON.stringify(window.__NUXT__); } catch (e) {}
  try { if (window.__INITIAL_STATE__) out.initial_state = JSON.stringify(window.__INITIAL_STATE__); } catch (e) {}
  const flight = [];
  document.querySelectorAll("script").forEach((s) => {
    const t = s.textContent || "";
    if (t.indexOf("self.__next_f.push") !== -1) flight.push(t);
  });
  if (flight.length) out.flight = flight.join("\n");
  document.querySelectorAll('script[type="application/ld+json"]').forEach((s) => {
    const t = s.textContent || "";
    if (t) out.json_ld.push(t);
  });
  return JSON.stringify(out);
})()`

// rawHarvest mirrors harvestScript's JSON output.
type rawHarvest struct {
	NextData     *string  `json:"next_data"`
	Nuxt         *string  `json:"nuxt"`
	InitialState *string  `json:"initial_state"`
	Flight       *string  `json:"flight"`
	JSONLD       []string `json:"json_ld"`
	Canvases     int      `json:"canvases"`
}

// parseHarvest converts raw harvest output into ExtractedData with caps
// applied. Returns nil when no candidate sources survived, so the response
// field can stay omitted.
func parseHarvest(rawJSON string) *domain.ExtractedData {
	var raw rawHarvest
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil
	}
	type candidate struct{ source, payload string }
	var cands []candidate
	if raw.NextData != nil {
		cands = append(cands, candidate{"__next_data", *raw.NextData})
	}
	if raw.Flight != nil {
		cands = append(cands, candidate{"next_flight", *raw.Flight})
	}
	if raw.Nuxt != nil {
		cands = append(cands, candidate{"__nuxt__", *raw.Nuxt})
	}
	if raw.InitialState != nil {
		cands = append(cands, candidate{"__initial_state__", *raw.InitialState})
	}
	for _, ld := range raw.JSONLD {
		if ld != "" {
			cands = append(cands, candidate{"json_ld", ld})
		}
	}

	total := 0
	out := &domain.ExtractedData{Canvases: raw.Canvases}
	for _, c := range cands {
		if len(out.Sources) >= maxHarvestSources || total >= maxHarvestTotalLen {
			out.Truncated = true
			break
		}
		payload := c.payload
		truncated := false
		if len(payload) > maxHarvestSourceLen {
			payload = payload[:maxHarvestSourceLen]
			truncated = true
		}
		if budget := maxHarvestTotalLen - total; len(payload) > budget {
			payload = payload[:budget]
			truncated = true
		}
		if payload == "" {
			continue
		}
		// Structured sources are real JSON and stay objects; flight chunks
		// are JS function calls, not JSON — keep them as raw text.
		var data any
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			data = payload
		}
		out.Sources = append(out.Sources, domain.DataSource{Source: c.source, Data: data})
		total += len(payload)
		if truncated {
			out.Truncated = true
		}
	}
	if len(out.Sources) == 0 {
		return nil
	}
	return out
}
