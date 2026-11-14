package scraper

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/bridge"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/recipe"
)

// newConsentTransport builds a transport over a real bridge State in a temp
// dir, so the allowlist under test is the one the server would actually read.
func newConsentTransport(t *testing.T) *BridgeTransport {
	t.Helper()
	st, err := bridge.NewState(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("bridge.NewState: %v", err)
	}
	server := bridge.NewServer(st, nil)
	return NewBridgeTransport(server, nil)
}

// TestRecipeFreeBorrowNeedsConsent is the security test for D30/D31.
//
// Removing the recipe removed the only authorization gate the borrow lane had,
// so this asserts the replacement: with no recipe, an origin the allowlist does
// not name is refused *before* anything reaches the browser. If this test ever
// passes with the consent check deleted, the lane is remote control for a
// signed-in browser and nothing says so.
func TestRecipeFreeBorrowNeedsConsent(t *testing.T) {
	tr := newConsentTransport(t)

	_, err := tr.Scrape(context.Background(), "https://intranet.example/board", nil, domain.ScrapeOptions{})
	if err == nil {
		t.Fatalf("want a refusal for an origin nobody allowed, got nil")
	}
	var consent *bridge.ConsentRequiredError
	if !errors.As(err, &consent) {
		t.Fatalf("want ConsentRequiredError, got %T: %v", err, err)
	}
	if consent.Origin != "https://intranet.example" {
		t.Fatalf("the refusal must name the origin a human has to allow, got %q", consent.Origin)
	}
	if !strings.Contains(err.Error(), "tomoshibi bridge allow") {
		t.Fatalf("the refusal must name the command that fixes it, got %q", err.Error())
	}
}

// TestAllowedOriginProceedsPastConsent: once allowed, the next failure is the
// missing browser, not consent. That distinction is what tells a caller whether
// to ask a human or to start the extension.
func TestAllowedOriginProceedsPastConsent(t *testing.T) {
	tr := newConsentTransport(t)
	if ok, err := tr.server.State().Allow("intranet.example"); err != nil || !ok {
		t.Fatalf("Allow: %v %v", ok, err)
	}

	_, err := tr.Scrape(context.Background(), "https://intranet.example/board", nil, domain.ScrapeOptions{})
	if err == nil {
		t.Fatalf("want an error from the absent browser")
	}
	var consent *bridge.ConsentRequiredError
	if errors.As(err, &consent) {
		t.Fatalf("consent should have been granted, got %v", err)
	}
	if !strings.Contains(err.Error(), "no browser connected") && !strings.Contains(err.Error(), "not paired") {
		t.Logf("error was %q (acceptable: the bridge reported its own state)", err)
	}
}

// TestRecipeIsItsOwnConsent: installing a pack for a host is the human act that
// authorizes it, so a recipe does not need the allowlist too.
func TestRecipeIsItsOwnConsent(t *testing.T) {
	tr := newConsentTransport(t)
	rec := mustRecipe(t, `
id: homelab
version: 1
match: { domains: ["router.home"] }
access: { lane: borrow, io: scripting, allow_private: true }
fixtures: [{ url: "http://router.home/", file: "f.html" }]
`)

	_, err := tr.Scrape(context.Background(), "http://router.home/admin", rec, domain.ScrapeOptions{})
	var consent *bridge.ConsentRequiredError
	if errors.As(err, &consent) {
		t.Fatalf("a recipe that declares the host must not need an allowlist entry, got %v", err)
	}
}

// TestAllowlistDoesNotNarrowRecipes pins the two-consent rule: a pack was
// installed on purpose, and allowing one unrelated host must not silently start
// refusing every pack. Two revocations for one decision is how a safety feature
// turns into a support ticket.
func TestAllowlistDoesNotNarrowRecipes(t *testing.T) {
	tr := newConsentTransport(t)
	if _, err := tr.server.State().Allow("other.example"); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	rec := mustRecipe(t, `
id: homelab
version: 1
match: { domains: ["router.home"] }
access: { lane: borrow, io: scripting, allow_private: true }
fixtures: [{ url: "http://router.home/", file: "f.html" }]
`)

	_, err := tr.Scrape(context.Background(), "http://router.home/admin", rec, domain.ScrapeOptions{})
	var consent *bridge.ConsentRequiredError
	if errors.As(err, &consent) {
		t.Fatalf("a pack's own declaration is consent; the allowlist must not veto it, got %v", err)
	}
}

// TestRecipeMustCoverItsHost closes a hole found while writing this: an explicit
// recipe id used to run against any host, which made "the recipe is the consent
// for these hosts" untrue.
func TestRecipeMustCoverItsHost(t *testing.T) {
	tr := newConsentTransport(t)
	rec := mustRecipe(t, `
id: homelab
version: 1
match: { domains: ["router.home"] }
access: { lane: borrow, io: scripting }
fixtures: [{ url: "http://router.home/", file: "f.html" }]
`)

	_, err := tr.Scrape(context.Background(), "https://elsewhere.example/x", rec, domain.ScrapeOptions{})
	if err == nil {
		t.Fatalf("want an error for a recipe applied to a host it does not declare")
	}
	if !strings.Contains(err.Error(), "does not cover host") {
		t.Fatalf("want a clear host-mismatch error, got %q", err)
	}
}

// TestPageBudgetRefusesTheNextRun: the budget is the difference between a
// research loop and a hundred tabs on someone's real session.
func TestPageBudgetRefusesTheNextRun(t *testing.T) {
	t.Setenv("BRIDGE_PAGES_PER_HOUR", "2")
	tr := newConsentTransport(t)

	if !tr.takeBudget(time.Now()) {
		t.Fatalf("first page should be within budget")
	}
	if !tr.takeBudget(time.Now()) {
		t.Fatalf("second page should be within budget")
	}
	if tr.takeBudget(time.Now()) {
		t.Fatalf("third page must be refused: budget is 2/hour")
	}
}

// TestPageBudgetDisabled: 0 is the documented opt-out.
func TestPageBudgetDisabled(t *testing.T) {
	t.Setenv("BRIDGE_PAGES_PER_HOUR", "0")
	tr := newConsentTransport(t)
	for i := 0; i < 50; i++ {
		if !tr.takeBudget(time.Now()) {
			t.Fatalf("budget 0 must mean unlimited, refused at %d", i)
		}
	}
}

// TestMayDriveMatchesTheRefusal keeps `transport: auto` honest: it escalates
// only where the real call would be allowed.
func TestMayDriveMatchesTheRefusal(t *testing.T) {
	tr := newConsentTransport(t)
	if tr.MayDrive(nil, "https://intranet.example/") {
		t.Fatalf("MayDrive must agree with consent: intranet.example is not allowed")
	}
	if !tr.MayDrive(nil, "https://html.duckduckgo.com/html?q=x") {
		t.Fatalf("MayDrive should allow a seeded search host")
	}
	if tr.MayDrive(nil, "not a url at all") {
		t.Fatalf("MayDrive must refuse something that is not a URL")
	}
}

// TestActionCapRefusesARunawayList: a pack is data and data can be wrong.
func TestActionCapRefusesARunawayList(t *testing.T) {
	tr := newConsentTransport(t)
	if ok, err := tr.server.State().Allow("intranet.example"); err != nil || !ok {
		t.Fatalf("Allow: %v %v", ok, err)
	}
	actions := make([]domain.Action, maxBridgeActions+1)
	for i := range actions {
		actions[i] = domain.Action{Type: "wait_ms", Ms: 1}
	}
	_, err := tr.Scrape(context.Background(), "https://intranet.example/", nil, domain.ScrapeOptions{Actions: actions})
	if err == nil || !strings.Contains(err.Error(), "more than the bridge will run") {
		t.Fatalf("want an action-cap refusal, got %v", err)
	}
}

var _ = recipe.LaneBorrow
