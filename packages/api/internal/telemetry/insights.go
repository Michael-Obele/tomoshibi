package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// EngineScore aggregates one native engine's attempts inside the window.
type EngineScore struct {
	Attempts      int       `json:"attempts"`
	OK            int       `json:"ok"`
	Empty         int       `json:"empty"`
	Blocked       int       `json:"blocked"`
	NotConfigured int       `json:"not_configured"`
	Timeouts      int       `json:"timeouts"`
	Errors        int       `json:"errors"`
	Results       int       `json:"results"`
	AvgLatencyMS  int64     `json:"avg_latency_ms"`
	MaxLatencyMS  int64     `json:"max_latency_ms"`
	LastSeen      time.Time `json:"last_seen"`
	LastError     string    `json:"last_error,omitempty"`
}

// ErrorSample is one search that failed outright (or ended with nothing),
// kept with its trace id so the log line can be found.
type ErrorSample struct {
	Time      time.Time `json:"ts"`
	TraceID   string    `json:"trace_id"`
	Query     string    `json:"query"`
	Backend   string    `json:"backend"`
	Error     string    `json:"error"`
	Fallbacks []string  `json:"fallbacks,omitempty"`
}

// Insights is the aggregated view returned by GET /v1/insights.
type Insights struct {
	WindowHours      int                     `json:"window_hours"`
	GeneratedAt      time.Time               `json:"generated_at"`
	Searches         int                     `json:"searches"`
	EmptySearches    int                     `json:"empty_searches"`
	FailedSearches   int                     `json:"failed_searches"`
	WeakGates        int                     `json:"weak_gates"`
	FallbackSearches int                     `json:"fallback_searches"`
	AvgLatencyMS     int64                   `json:"avg_latency_ms"`
	P50LatencyMS     int64                   `json:"p50_latency_ms"`
	P95LatencyMS     int64                   `json:"p95_latency_ms"`
	Results          int                     `json:"results"`
	Backends         map[string]int          `json:"backends"`
	Engines          map[string]*EngineScore `json:"engines"`
	RecentErrors     []ErrorSample           `json:"recent_errors"`
}

const recentErrorsCap = 20

// Read aggregates every event file in dir whose timestamp falls inside
// [now-window, now]. A missing directory yields an empty report, not an
// error, so /v1/insights works before the first search is recorded.
func Read(dir string, window time.Duration) (Insights, error) {
	ins := Insights{
		WindowHours: int(window.Hours()),
		GeneratedAt: time.Now().UTC(),
		Backends:    map[string]int{},
		Engines:     map[string]*EngineScore{},
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ins, nil
		}
		return ins, err
	}

	cutoff := time.Now().UTC().Add(-window)
	var latencies []int64

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return ins, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var probe struct {
				Backend string `json:"backend"`
				Engine  string `json:"engine"`
			}
			if err := json.Unmarshal(line, &probe); err != nil {
				continue
			}
			if probe.Engine != "" {
				var ev EngineEvent
				if err := json.Unmarshal(line, &ev); err != nil || !ev.Time.After(cutoff) {
					continue
				}
				accumulateEngine(&ins, ev)
				continue
			}
			var ev SearchEvent
			if err := json.Unmarshal(line, &ev); err != nil || !ev.Time.After(cutoff) {
				continue
			}
			accumulateSearch(&ins, ev, &latencies)
		}
		scanErr := sc.Err()
		closeErr := f.Close()
		if scanErr != nil {
			return ins, scanErr
		}
		if closeErr != nil {
			return ins, closeErr
		}
	}

	sort.Slice(ins.RecentErrors, func(i, j int) bool {
		return ins.RecentErrors[i].Time.After(ins.RecentErrors[j].Time)
	})
	if len(ins.RecentErrors) > recentErrorsCap {
		ins.RecentErrors = ins.RecentErrors[:recentErrorsCap]
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	if n := len(latencies); n > 0 {
		var sum int64
		for _, l := range latencies {
			sum += l
		}
		ins.AvgLatencyMS = sum / int64(n)
		ins.P50LatencyMS = percentile(latencies, 0.50)
		ins.P95LatencyMS = percentile(latencies, 0.95)
	}
	return ins, nil
}

func accumulateSearch(ins *Insights, ev SearchEvent, latencies *[]int64) {
	ins.Searches++
	ins.Results += ev.Results
	*latencies = append(*latencies, ev.LatencyMS)
	backend := ev.Backend
	if backend == "" {
		backend = "unknown"
	}
	ins.Backends[backend]++
	if ev.Weak {
		ins.WeakGates++
	}
	if len(ev.Fallbacks) > 0 {
		ins.FallbackSearches++
	}
	switch {
	case ev.Error != "":
		ins.FailedSearches++
	case ev.Results == 0:
		ins.EmptySearches++
	}
	if ev.Error != "" || ev.Results == 0 {
		ins.RecentErrors = append(ins.RecentErrors, ErrorSample{
			Time:      ev.Time,
			TraceID:   ev.TraceID,
			Query:     ev.Query,
			Backend:   backend,
			Error:     ev.Error,
			Fallbacks: ev.Fallbacks,
		})
	}
}

func accumulateEngine(ins *Insights, ev EngineEvent) {
	score, ok := ins.Engines[ev.Engine]
	if !ok {
		score = &EngineScore{}
		ins.Engines[ev.Engine] = score
	}
	score.Attempts++
	score.Results += ev.Results
	if ev.LatencyMS > score.MaxLatencyMS {
		score.MaxLatencyMS = ev.LatencyMS
	}
	score.AvgLatencyMS = (score.AvgLatencyMS*int64(score.Attempts-1) + ev.LatencyMS) / int64(score.Attempts)
	if ev.Time.After(score.LastSeen) {
		score.LastSeen = ev.Time
	}
	switch ev.Status {
	case "ok":
		score.OK++
	case "empty":
		score.Empty++
	case "blocked":
		score.Blocked++
	case "not_configured":
		score.NotConfigured++
	case "timeout":
		score.Timeouts++
	default:
		score.Errors++
	}
	if ev.Status != "ok" && ev.Status != "empty" && ev.Detail != "" {
		score.LastError = ev.Status + ": " + ev.Detail
	}
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p*float64(len(sorted))+0.9999) - 1 // round up
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
