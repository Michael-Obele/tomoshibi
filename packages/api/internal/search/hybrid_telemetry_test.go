package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/telemetry"
)

// recStub captures telemetry for assertions (implements telemetry.Recorder).
type recStub struct {
	searches []telemetry.SearchEvent
	engines  []telemetry.EngineEvent
}

func (r *recStub) RecordSearch(e telemetry.SearchEvent) {
	r.searches = append(r.searches, e)
}

func (r *recStub) RecordEngine(e telemetry.EngineEvent) {
	r.engines = append(r.engines, e)
}

// TestHybridRecordsSearchTelemetry: a search that hits the weak gate records
// one event with the trace id, the fallback reason and the outcome.
func TestHybridRecordsSearchTelemetry(t *testing.T) {
	rec := &recStub{}
	weak := concentrated(6, "en.wikipedia.org")
	strong := diverse(6)
	h := &HybridService{
		services: []Service{
			&stubService{results: weak, count: 6},
			&stubService{results: strong, count: 6},
		},
		rec: rec,
	}

	if _, _, err := h.Search(context.Background(), SearchOptions{Query: "q"}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(rec.searches) != 1 {
		t.Fatalf("recorded %d search events, want 1", len(rec.searches))
	}
	ev := rec.searches[0]
	if ev.TraceID == "" {
		t.Error("TraceID is empty — the join key to engine events must be set")
	}
	if !ev.Weak {
		t.Error("Weak = false, want true (the gate fired)")
	}
	if len(ev.Fallbacks) != 1 || !strings.Contains(ev.Fallbacks[0], "weak") {
		t.Errorf("Fallbacks = %v, want exactly the weak skip", ev.Fallbacks)
	}
	if ev.Results != 6 {
		t.Errorf("Results = %d, want 6", ev.Results)
	}
	if ev.Query != "q" {
		t.Errorf("Query = %q, want %q", ev.Query, "q")
	}
	if ev.Error != "" {
		t.Errorf("Error = %q, want empty", ev.Error)
	}
}

// TestHybridRecordsFailedSearch: a chain that answers nothing still records —
// with the error text and every backend's reason — because that is exactly
// what /v1/insights' recent_errors is for.
func TestHybridRecordsFailedSearch(t *testing.T) {
	rec := &recStub{}
	h := &HybridService{
		services: []Service{&stubService{err: errors.New("down")}},
		rec:      rec,
	}

	if _, _, err := h.Search(context.Background(), SearchOptions{Query: "q"}); err == nil {
		t.Fatal("Search() error = nil, want the backend error")
	}
	if len(rec.searches) != 1 {
		t.Fatalf("recorded %d search events, want 1", len(rec.searches))
	}
	ev := rec.searches[0]
	if !strings.Contains(ev.Error, "down") {
		t.Errorf("Error = %q, want it to contain 'down'", ev.Error)
	}
	if ev.Backend != "none" {
		t.Errorf("Backend = %q, want none", ev.Backend)
	}
	if len(ev.Fallbacks) != 1 || !strings.Contains(ev.Fallbacks[0], "error") {
		t.Errorf("Fallbacks = %v, want the backend error reason", ev.Fallbacks)
	}
}
