package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStoreAndReadRoundTrip covers the whole path: record search + engine
// events, then aggregate them the way /v1/insights would.
func TestStoreAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, 7)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	store.RecordSearch(SearchEvent{
		TraceID: "abc123", Query: "q1", Backend: "native",
		Results: 10, LatencyMS: 120,
	})
	store.RecordSearch(SearchEvent{
		TraceID: "def456", Query: "q2", Backend: "searxng",
		Fallbacks: []string{"native: weak"}, Weak: true, Results: 0, LatencyMS: 900,
	})
	store.RecordSearch(SearchEvent{
		TraceID: "ghi789", Query: "q3", Backend: "none",
		Error: "all backends failed", LatencyMS: 5,
	})
	store.RecordEngine(EngineEvent{
		TraceID: "abc123", Engine: "wikipedia", Status: "ok", Results: 10, LatencyMS: 80,
	})
	store.RecordEngine(EngineEvent{
		TraceID: "abc123", Engine: "mojeek", Status: "blocked", Detail: "captcha", LatencyMS: 700,
	})
	store.RecordEngine(EngineEvent{
		TraceID: "abc123", Engine: "ddg", Status: "timeout", LatencyMS: 5000,
	})
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	ins, err := Read(dir, time.Hour)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if ins.Searches != 3 {
		t.Errorf("Searches = %d, want 3", ins.Searches)
	}
	if ins.Backends["native"] != 1 || ins.Backends["searxng"] != 1 || ins.Backends["none"] != 1 {
		t.Errorf("Backends = %v, want one each of native/searxng/none", ins.Backends)
	}
	if ins.WeakGates != 1 || ins.FallbackSearches != 1 {
		t.Errorf("WeakGates/FallbackSearches = %d/%d, want 1/1", ins.WeakGates, ins.FallbackSearches)
	}
	if ins.FailedSearches != 1 || ins.EmptySearches != 1 {
		t.Errorf("Failed/Empty = %d/%d, want 1/1", ins.FailedSearches, ins.EmptySearches)
	}
	if ins.Results != 10 {
		t.Errorf("Results = %d, want 10", ins.Results)
	}
	if ins.P50LatencyMS != 120 || ins.P95LatencyMS != 900 {
		t.Errorf("P50/P95 = %d/%d, want 120/900", ins.P50LatencyMS, ins.P95LatencyMS)
	}
	if len(ins.RecentErrors) != 2 {
		t.Errorf("RecentErrors = %d, want 2 (the empty + failed searches)", len(ins.RecentErrors))
	}

	wiki := ins.Engines["wikipedia"]
	if wiki == nil || wiki.OK != 1 || wiki.Attempts != 1 || wiki.Results != 10 {
		t.Errorf("wikipedia score = %+v, want ok=1 attempts=1 results=10", wiki)
	}
	mojeek := ins.Engines["mojeek"]
	if mojeek == nil || mojeek.Blocked != 1 || mojeek.LastError != "blocked: captcha" {
		t.Errorf("mojeek score = %+v, want blocked=1 with 'blocked: captcha'", mojeek)
	}
	if ddg := ins.Engines["ddg"]; ddg == nil || ddg.Timeouts != 1 {
		t.Errorf("ddg score = %+v, want timeouts=1", ddg)
	}
}

// TestReadWindowSkipsOld: events outside the window are not counted.
func TestReadWindowSkipsOld(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	store.RecordSearch(SearchEvent{
		Time:    time.Now().UTC().Add(-48 * time.Hour),
		Backend: "native", Results: 3,
	})
	store.RecordSearch(SearchEvent{Backend: "native", Results: 4})
	store.Close()

	ins, err := Read(dir, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Searches != 1 {
		t.Errorf("Searches = %d, want 1 (the recent one)", ins.Searches)
	}
}

// TestReadMissingDir: no telemetry yet is not an error.
func TestReadMissingDir(t *testing.T) {
	ins, err := Read(filepath.Join(t.TempDir(), "nope"), time.Hour)
	if err != nil {
		t.Fatalf("Read() on missing dir error = %v", err)
	}
	if ins.Searches != 0 || ins.Engines == nil {
		t.Errorf("expected an empty report, got %+v", ins)
	}
}

// TestPruneRemovesOldFiles: startup retention deletes stale day files.
func TestPruneRemovesOldFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "search-2020-01-01.jsonl")
	if err := os.WriteFile(old, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir, 1); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected the 2020 file to be pruned, stat err = %v", err)
	}
}

func TestNopRecorder(t *testing.T) {
	var r Recorder = Nop{}
	r.RecordSearch(SearchEvent{Backend: "native"})
	r.RecordEngine(EngineEvent{Engine: "ddg", Status: "ok"})
}

func TestTraceIDRoundTrip(t *testing.T) {
	id := NewTraceID()
	if len(id) != 16 {
		t.Errorf("len(NewTraceID()) = %d, want 16", len(id))
	}
	ctx := WithTraceID(context.Background(), id)
	if got := TraceID(ctx); got != id {
		t.Errorf("TraceID() = %q, want %q", got, id)
	}
	if got := TraceID(context.Background()); got != "" {
		t.Errorf("TraceID() on bare ctx = %q, want empty", got)
	}
}
