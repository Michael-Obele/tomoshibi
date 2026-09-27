// Package telemetry records what search actually did — one event per search
// (who answered, what it cost, what fell through) and one per native engine
// attempt (ok/empty/blocked/error with latency) — as append-only JSONL files
// in a local directory, then aggregates them for GET /v1/insights.
//
// Local-only by design: this package makes no network calls and exports
// nothing. Files live under TELEMETRY_DIR (default data/telemetry, gitignored,
// bind-mounted in docker-compose) and rotate daily; TELEMETRY_RETAIN_DAYS
// prunes them on startup.
//
// The point is a continuous engine scorecard: the canary gate tells you what
// engines do under a test query, these files tell you what they do under real
// traffic — which engines answer, which block, how often the chain falls
// through, and whether the weak-result gate is earning its keep.
package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// SearchEvent is one search as the chain experienced it.
type SearchEvent struct {
	Time      time.Time `json:"ts"`
	TraceID   string    `json:"trace_id"`
	Query     string    `json:"query"`
	Category  string    `json:"category,omitempty"`
	Backend   string    `json:"backend"`             // native | searxng | stealth | brave | none
	Fallbacks []string  `json:"fallbacks,omitempty"` // "native: weak", "searxng: error: ..."
	Engines   []string  `json:"engines,omitempty"`   // native engines that contributed
	Results   int       `json:"results"`
	LatencyMS int64     `json:"latency_ms"`
	Weak      bool      `json:"weak,omitempty"` // the weak-result gate fired
	Error     string    `json:"error,omitempty"`
}

// EngineEvent is one native engine's answer to one search.
type EngineEvent struct {
	Time      time.Time `json:"ts"`
	TraceID   string    `json:"trace_id"`
	Engine    string    `json:"engine"`
	Status    string    `json:"status"` // ok | empty | blocked | not_configured | timeout | error
	Detail    string    `json:"detail,omitempty"`
	Results   int       `json:"results"`
	LatencyMS int64     `json:"latency_ms"`
}

// Recorder is the seam every recording point depends on (nil is not allowed —
// use Nop, so a typed-nil can never panic a hot path).
type Recorder interface {
	RecordSearch(SearchEvent)
	RecordEngine(EngineEvent)
}

// Nop is the recorder used when telemetry is disabled.
type Nop struct{}

func (Nop) RecordSearch(SearchEvent) {}
func (Nop) RecordEngine(EngineEvent) {}

type ctxKeyTrace struct{}

// WithTraceID attaches a trace id to the request context so hybrid (who
// creates it) and the native layer (who records per-engine outcomes) write
// events that join on the same id.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyTrace{}, id)
}

// TraceID returns the id set by WithTraceID, or "".
func TraceID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyTrace{}).(string)
	return id
}

// NewTraceID returns a 16-hex-char id — short enough to paste from a log,
// unique enough to join events.
func NewTraceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Store appends events to daily JSONL files (search-YYYY-MM-DD.jsonl,
// engine-YYYY-MM-DD.jsonl) under dir. Safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	dir    string
	retain time.Duration
	files  map[string]*os.File
	closed bool
}

// New opens (creating if needed) a store in dir and prunes files older than
// retainDays (0 = keep forever).
func New(dir string, retainDays int) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("telemetry dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create telemetry dir: %w", err)
	}
	s := &Store{dir: dir, files: map[string]*os.File{}}
	if retainDays > 0 {
		s.retain = time.Duration(retainDays) * 24 * time.Hour
		s.prune()
	}
	return s, nil
}

// RecordSearch appends one search event (Time is filled when zero).
func (s *Store) RecordSearch(e SearchEvent) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	s.append("search", e)
}

// RecordEngine appends one engine event (Time is filled when zero).
func (s *Store) RecordEngine(e EngineEvent) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	s.append("engine", e)
}

func (s *Store) append(kind string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	day := time.Now().UTC().Format("2006-01-02")
	key := kind + "-" + day
	f, ok := s.files[key]
	if !ok {
		var err error
		f, err = os.OpenFile(filepath.Join(s.dir, key+".jsonl"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			s.logErr(kind, err)
			return
		}
		s.files[key] = f
	}
	if err := json.NewEncoder(f).Encode(v); err != nil {
		s.logErr(kind, err)
	}
}

// prune deletes event files older than the retention window.
func (s *Store) prune() {
	cutoff := time.Now().UTC().Add(-s.retain)
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		s.logErr("prune", err)
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || len(name) < len("2006-01-02.jsonl") {
			continue
		}
		day, ok := splitDay(name)
		if !ok {
			continue
		}
		t, err := time.Parse("2006-01-02", day)
		if err != nil {
			continue
		}
		if t.Add(24 * time.Hour).Before(cutoff) {
			if err := os.Remove(filepath.Join(s.dir, name)); err != nil {
				s.logErr("prune", err)
			}
		}
	}
}

// splitDay pulls "2006-01-02" out of "search-2006-01-02.jsonl".
func splitDay(name string) (string, bool) {
	ext := ".jsonl"
	if len(name) <= len(ext) || name[len(name)-len(ext):] != ext {
		return "", false
	}
	base := name[:len(name)-len(ext)]
	for i := 0; i+len("2006-01-02") <= len(base); i++ {
		cand := base[i : i+len("2006-01-02")]
		if _, err := time.Parse("2006-01-02", cand); err == nil {
			return cand, true
		}
	}
	return "", false
}

// Close flushes and closes open files.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var first error
	for k, f := range s.files {
		if err := f.Close(); err != nil && first == nil {
			first = err
		}
		delete(s.files, k)
	}
	return first
}

// logErr reports a telemetry write failure without ever failing the request
// that triggered it — telemetry is best-effort by design.
func (s *Store) logErr(op string, err error) {
	if logger.Log != nil {
		logger.Log.Warn("telemetry write failed", "op", op, "error", err)
	}
}
