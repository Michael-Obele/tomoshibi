//go:build canary

package engines

// canary_test.go — live per-engine health probe (plan/tomoshi-search M2).
//
//	go test -tags=canary ./internal/search/engines/ -run TestCanary -v
//
// Runs 3 real queries per engine against live upstreams and prints
// {ok, empty, blocked, err, ms} plus a non-empty-rate summary. CI never
// runs this (build tag); schedule it via Asynq cron. The gate metric is
// >=90% non-empty over 7 days, otherwise the engine escalates to a proxy
// group or leaves the roster.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// loadDotEnv walks up from the test's CWD (the package dir — go test does
// not run at the repo root) to the repo root, loading any .env found.
// godotenv never overrides already-set variables, so this is safe alongside
// a real environment.
func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for i := 0; i < 8 && dir != "" && dir != "/"; i++ {
		_ = godotenv.Load(filepath.Join(dir, ".env"))
		dir = filepath.Dir(dir)
	}
}

var canaryQueries = map[string][]string{
	"general": {"web scraping", "golang concurrency", "best pizza recipe"},
	"news":    {"openai devday", "golang release", "web search engines"},
	"it":      {"golang generics", "rust ownership", "postgres indexing"},
}

func TestCanary(t *testing.T) {
	if testing.Short() {
		t.Skip("live canary skipped in -short mode")
	}
	// Load the nearest .env so API-key engines (brave) and Webshare
	// credentials are present on local runs.
	loadDotEnv()

	reg, err := LoadRegistry("")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	pool := NewProxyPool(ProxyOptions{
		WebshareAPIKey:   os.Getenv("WEBSHARE_API_KEY"),
		WebshareProxyURL: os.Getenv("WEBSHARE_PROXY_URL"),
		Tier1:            os.Getenv("SEARCH_PROXY_TIER1"),
		Tier2:            os.Getenv("SEARCH_PROXY_TIER2"),
		BudgetGB:         os.Getenv("SEARCH_PROXY_BUDGET_GB"),
	})

	// CANARY_ENGINES=ddg,marginalia limits the sweep (fast iteration).
	// An explicit filter may also name disabled opt-ins (google, startpage)
	// so their proxy experiments can be probed without enabling them.
	allow := map[string]bool{}
	for _, n := range strings.Split(os.Getenv("CANARY_ENGINES"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			allow[n] = true
		}
	}
	specs := make([]*Spec, 0, len(reg.Specs()))
	for _, spec := range reg.Specs() {
		if len(allow) > 0 {
			if allow[spec.Name] {
				specs = append(specs, spec)
			}
		} else if spec.IsEnabled() {
			specs = append(specs, spec)
		}
	}
	engs := newEngines(specs, pool)

	type stat struct {
		ok, empty, blocked, fail int
		elapsed                  time.Duration
	}
	stats := map[string]*stat{}

	for _, spec := range specs {
		var e Engine
		for _, cand := range engs {
			if cand.Name() == spec.Name {
				e = cand
				break
			}
		}
		if e == nil {
			continue
		}
		cat := spec.Categories[0]
		queries := canaryQueries[cat]
		if len(queries) == 0 {
			queries = canaryQueries["general"]
		}
		st := stats[spec.Name]
		if st == nil {
			st = &stat{}
			stats[spec.Name] = st
		}
		for _, q := range queries {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			start := time.Now()
			res, err := e.Search(ctx, Query{Q: q, Pageno: 1, Language: "en"})
			d := time.Since(start)
			cancel()
			st.elapsed += d
			switch {
			case err == nil && len(res) > 0:
				st.ok++
				t.Logf("%-14s ok     %2d hits  %5dms  q=%q", spec.Name, len(res), d.Milliseconds(), q)
			case err == nil:
				st.empty++
				t.Logf("%-14s EMPTY  %5dms  q=%q", spec.Name, d.Milliseconds(), q)
			case errors.Is(err, ErrBlocked):
				st.blocked++
				t.Logf("%-14s BLOCKED %5dms  q=%q  %v", spec.Name, d.Milliseconds(), q, err)
			default:
				st.fail++
				t.Logf("%-14s FAIL   %5dms  q=%q  %v", spec.Name, d.Milliseconds(), q, err)
			}
			time.Sleep(750 * time.Millisecond) // polite pacing (DDG anomaly-pages on fast repeats)
		}
	}

	t.Log("--- summary (gate: >=90% non-empty) ---")
	weak := 0
	for _, spec := range specs {
		st := stats[spec.Name]
		if st == nil {
			continue
		}
		attempts := st.ok + st.empty + st.blocked + st.fail
		rate := float64(st.ok) / float64(attempts) * 100
		flag := ""
		if rate < 90 {
			flag = "  <-- below gate"
			weak++
		}
		t.Logf("%-14s ok=%d empty=%d blocked=%d fail=%d  %3.0f%%%s",
			spec.Name, st.ok, st.empty, st.blocked, st.fail, rate, flag)
	}
	if pool.BudgetExhausted() {
		t.Log("proxy budget exhausted during canary — engines degraded to direct")
	}
	if weak > 0 {
		t.Logf("%d engine(s) below the 90%% gate: assign a proxy group or drop from roster", weak)
	}
}
