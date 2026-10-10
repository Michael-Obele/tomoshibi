package scraper

import (
	"fmt"
	"strings"
	"testing"
)

func jsonLDArray(n, size int) string {
	entries := make([]string, 0, n)
	for i := range n {
		entries = append(entries, fmt.Sprintf("%q", fmt.Sprintf("payload%d%s", i, strings.Repeat("a", size))))
	}
	return fmt.Sprintf(`{"json_ld":[%s],"canvases":1}`, strings.Join(entries, ","))
}

func TestParseHarvest(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		wantSources   int
		wantFirstSrc  string
		wantDataIsStr bool
		wantObject    bool
		wantCanvases  int
		wantTruncated bool
	}{
		{
			name:        "no candidates returns nil",
			raw:         `{"next_data":null,"nuxt":null,"initial_state":null,"flight":null,"json_ld":[],"canvases":0}`,
			wantSources: 0,
		},
		{
			name:         "json-ld parsed as object",
			raw:          `{"json_ld":["{\"@context\":\"https://schema.org\"}"],"canvases":3}`,
			wantSources:  1,
			wantFirstSrc: "json_ld",
			wantObject:   true,
			wantCanvases: 3,
		},
		{
			name:         "next_data parsed as object",
			raw:          `{"next_data":"{\"props\":{\"page\":1}}","json_ld":[]}`,
			wantSources:  1,
			wantFirstSrc: "__next_data",
			wantObject:   true,
		},
		{
			name:          "flight chunks stay raw string",
			raw:           `{"flight":"self.__next_f.push([1,\"chunk\"])","json_ld":[]}`,
			wantSources:   1,
			wantFirstSrc:  "next_flight",
			wantDataIsStr: true,
		},
		{
			name:          "oversized source truncated",
			raw:           fmt.Sprintf(`{"json_ld":["%s"],"canvases":1}`, strings.Repeat("a", maxHarvestSourceLen+10)),
			wantSources:   1,
			wantFirstSrc:  "json_ld",
			wantDataIsStr: true,
			wantCanvases:  1,
			wantTruncated: true,
		},
		{
			name:          "source count capped at 8",
			raw:           jsonLDArray(10, 4),
			wantSources:   maxHarvestSources,
			wantFirstSrc:  "json_ld",
			wantCanvases:  1,
			wantTruncated: true,
		},
		{
			name:          "total budget capped at 512KB",
			raw:           jsonLDArray(5, maxHarvestSourceLen),
			wantSources:   4,
			wantFirstSrc:  "json_ld",
			wantCanvases:  1,
			wantTruncated: true,
		},
		{
			name:        "invalid json returns nil",
			raw:         `not json`,
			wantSources: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHarvest(tt.raw)
			if tt.wantSources == 0 {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected ExtractedData, got nil")
			}
			if len(got.Sources) != tt.wantSources {
				t.Errorf("sources = %d, want %d", len(got.Sources), tt.wantSources)
			}
			if got.Sources[0].Source != tt.wantFirstSrc {
				t.Errorf("first source = %q, want %q", got.Sources[0].Source, tt.wantFirstSrc)
			}
			if tt.wantDataIsStr {
				if _, ok := got.Sources[0].Data.(string); !ok {
					t.Errorf("data type = %T, want string", got.Sources[0].Data)
				}
			}
			if tt.wantObject {
				if _, ok := got.Sources[0].Data.(map[string]any); !ok {
					t.Errorf("data type = %T, want object", got.Sources[0].Data)
				}
			}
			if got.Canvases != tt.wantCanvases {
				t.Errorf("canvases = %d, want %d", got.Canvases, tt.wantCanvases)
			}
			if got.Truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", got.Truncated, tt.wantTruncated)
			}
		})
	}
}
