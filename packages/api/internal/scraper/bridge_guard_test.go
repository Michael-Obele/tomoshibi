package scraper

import (
	"context"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/recipe"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
)

// TestBorrowLaneSSRFGuard: the borrow lane drives a real browser on the user's
// own network, so pointing it at a private address has to be refused unless the
// recipe declares that host AND opts in. TestMain disables the guard for the
// rest of the package, so this test turns it back on explicitly.
func TestBorrowLaneSSRFGuard(t *testing.T) {
	t.Setenv(safeurl.AllowPrivateEnv, "false")

	// Match and Access are unexported, so the variants are built through YAML,
	// which is how a real recipe arrives anyway.
	strict := mustRecipe(t, `
id: homelab
version: 1
match: { domains: ["router.home"] }
access: { lane: borrow, io: scripting }
fixtures: [{ url: "http://router.home/", file: "f.html" }]
`)
	opted := mustRecipe(t, `
id: homelab
version: 1
match: { domains: ["router.home"] }
access: { lane: borrow, io: scripting, allow_private: true }
fixtures: [{ url: "http://router.home/", file: "f.html" }]
`)
	other := mustRecipe(t, `
id: homelab
version: 1
match: { domains: ["elsewhere.example"] }
access: { lane: borrow, io: scripting, allow_private: true }
fixtures: [{ url: "http://elsewhere.example/", file: "f.html" }]
`)

	// No bridge configured, so a request that gets past the guard fails with
	// the transport error rather than the SSRF error. The guard's decision is
	// what this test reads.
	tr := &BridgeTransport{}
	cases := []struct {
		name       string
		rec        *recipe.Recipe
		url        string
		wantSSRF   bool
		wantErrSub string
	}{
		{"private host without the opt-in", strict, "http://192.168.1.1/admin", true, "private"},
		// The declared host itself: safeurl is skipped, so the error that comes
		// back is about the missing bridge, not about the address.
		{"declared host with the opt-in", opted, "http://router.home/admin", false, "bridge"},
		{"undeclared host even with the opt-in", other, "http://192.168.1.1/admin", true, "private"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tr.Scrape(context.Background(), tc.url, tc.rec, domain.ScrapeOptions{})
			if err == nil {
				t.Fatalf("want an error, got nil")
			}
			got := err.Error()
			if tc.wantSSRF {
				if !strings.Contains(got, "private") && !strings.Contains(got, "blocked") {
					t.Fatalf("want the SSRF guard to refuse, got %q", got)
				}
				return
			}
			if strings.Contains(got, "private") || strings.Contains(got, "blocked") {
				t.Fatalf("guard should have allowed this target, got %q", got)
			}
		})
	}
}

func mustRecipe(t *testing.T, yaml string) *recipe.Recipe {
	t.Helper()
	r, err := recipe.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return r
}
