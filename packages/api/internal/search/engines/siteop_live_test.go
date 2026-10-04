package engines

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSiteOperatorLive proves the operator works against a real upstream, not
// just against our own string handling: with a domain filter pushed into the
// query, the engine must return in-domain hits without us discarding most of
// the page in post-filtering.
//
// Live and therefore opt-in — engine HTML changes without notice and CI must
// not depend on a third party. Run with:
//
//	TOMOSHI_LIVE=1 go test ./internal/search/engines/ -run SiteOperatorLive -v
func TestSiteOperatorLive(t *testing.T) {
	if os.Getenv("TOMOSHI_LIVE") != "1" {
		t.Skip("live engine test; set TOMOSHI_LIVE=1")
	}
	if testing.Short() {
		t.Skip("live engine test skipped in -short mode")
	}

	reg, err := LoadRegistry("")
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	// Probe only engines that declare the capability. Bing is excluded on
	// purpose: it documented site: but ignored it in practice (measured
	// 2026-10-04: 10 results, 0 in-domain), which is why its flag is off.
	// Probing it here would assert the behaviour we deliberately removed.
	var probes []Engine
	for _, name := range []string{"marginalia", "brave", "ddg"} {
		if reg.Spec(name).SiteOperator {
			probes = append(probes, newEngines([]*Spec{reg.Spec(name)}, nil)...)
		}
	}
	if len(probes) == 0 {
		t.Skip("no engine declares site_operator")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ran, inDomain := 0, 0
	for _, e := range probes {
		res, err := e.Search(ctx, Query{
			Q:              "http router",
			IncludeDomains: []string{"go.dev"},
		})
		if err != nil {
			t.Logf("engine %s unavailable: %v", e.Name(), err)
			continue
		}
		ran++
		for _, r := range res {
			if strings.Contains(r.URL, "go.dev") {
				inDomain++
			}
		}
		t.Logf("engine %s: %d results, %d in-domain", e.Name(), len(res), inDomain)
	}

	if ran == 0 {
		t.Skip("no site_operator engine answered; egress is likely blocked")
	}
	if inDomain == 0 {
		t.Error("site: injection returned no in-domain results")
	}
}
