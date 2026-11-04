package scraper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "pages", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(body)
}

// paddedArticle turns the article fixture into a realistic long page. The
// point of the padding is the false-positive guard: a large document that
// quotes a challenge marker is content, not a wall.
func paddedArticle(t *testing.T) string {
	t.Helper()
	article := fixture(t, "article_about_cloudflare.html")
	return strings.Replace(article, "<!--PAD-->", strings.Repeat("<p>More detail about the signal stack and how it is combined.</p>", 800), 1)
}

func TestDetectChallenge_Fixtures(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		status     int
		wantVendor string
	}{
		{"cloudflare interstitial", fixture(t, "cloudflare_challenge.html"), 200, "cloudflare"},
		{"datadome interstitial", fixture(t, "datadome_challenge.html"), 200, "datadome"},
		{"perimeterx interstitial", fixture(t, "perimeterx_challenge.html"), 200, "perimeterx"},
		{"large article quoting a marker is content", paddedArticle(t), 200, ""},
		{"server-rendered page is content", fixture(t, "nextjs_ssr.html"), 200, ""},
		{"empty body is not a challenge", "", 200, ""},
		{"weak marker with an error status is a challenge", "<html><head><title>Access Denied</title></head><body>nope</body></html>", 403, "akamai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectChallenge([]byte(tt.body), tt.status)
			if tt.wantVendor == "" {
				if got != nil {
					t.Fatalf("detected %q (%q), want content", got.Vendor, got.Marker)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected a %s challenge, got content", tt.wantVendor)
			}
			if got.Vendor != tt.wantVendor {
				t.Fatalf("vendor = %q, want %q", got.Vendor, tt.wantVendor)
			}
		})
	}
}

func TestShouldUseDynamic_Fixtures(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"react shell needs a browser", fixture(t, "spa_shell.html"), true},
		{"server-rendered next page does not", fixture(t, "nextjs_ssr.html"), false},
		{"long article does not", paddedArticle(t), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldUseDynamic(tt.body); got != tt.want {
				t.Fatalf("ShouldUseDynamic = %v, want %v", got, tt.want)
			}
		})
	}
}

func challengeResult(t *testing.T) *domain.ScrapeResult {
	t.Helper()
	return &domain.ScrapeResult{
		URL:      "https://example.com",
		HTML:     fixture(t, "cloudflare_challenge.html"),
		Markdown: "Just a moment...",
		Metadata: map[string]string{"engine": "colly"},
	}
}

func TestService_SmartEscalatesChallenge(t *testing.T) {
	static := &mockScraper{result: challengeResult(t)}
	dynamic := &mockScraper{result: newMockResult("chromedp")}
	svc := NewService(static, dynamic, nil)

	result, err := svc.Scrape(context.Background(), "https://example.com", "smart", domain.ScrapeOptions{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if got := result.Metadata["engine"]; got != "chromedp" {
		t.Fatalf("engine = %q, want chromedp (the challenge should force a browser retry)", got)
	}
}

func TestService_SmartFailsWhenBothEnginesAreBlocked(t *testing.T) {
	static := &mockScraper{result: challengeResult(t)}
	dynamic := &mockScraper{result: challengeResult(t)}
	svc := NewService(static, dynamic, nil)

	_, err := svc.Scrape(context.Background(), "https://example.com", "smart", domain.ScrapeOptions{})
	if err == nil {
		t.Fatal("expected a BlockedError, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("error = %T %v, want *BlockedError", err, err)
	}
	if blocked.Vendor != "cloudflare" {
		t.Fatalf("vendor = %q, want cloudflare", blocked.Vendor)
	}
}

func TestService_StaticModeDoesNotEscalate(t *testing.T) {
	static := &mockScraper{result: challengeResult(t)}
	dynamic := &countingScraper{result: newMockResult("chromedp")}
	svc := NewService(static, dynamic, nil)

	_, err := svc.Scrape(context.Background(), "https://example.com", "static", domain.ScrapeOptions{})
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("error = %T %v, want *BlockedError", err, err)
	}
	if got := dynamic.calls.Load(); got != 0 {
		t.Fatalf("dynamic scraper called %d times in static mode, want 0", got)
	}
}

type fakeSolver struct {
	got      domain.ChallengeRequest
	solution *domain.ChallengeSolution
	err      error
}

func (f *fakeSolver) Solve(_ context.Context, req domain.ChallengeRequest) (*domain.ChallengeSolution, error) {
	f.got = req
	return f.solution, f.err
}

func TestService_SolverRecoversContent(t *testing.T) {
	solver := &fakeSolver{solution: &domain.ChallengeSolution{
		HTML: "<html><body><h1>Solved page</h1>" + makeStaticHTML() + "</body></html>",
	}}
	svc := NewService(&mockScraper{result: challengeResult(t)}, nil, nil, WithSolver(solver))

	result, err := svc.Scrape(context.Background(), "https://example.com", "static", domain.ScrapeOptions{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Metadata["challenge_solved"] != "true" {
		t.Fatalf("challenge_solved = %q, want true", result.Metadata["challenge_solved"])
	}
	if !strings.Contains(result.Markdown, "Solved page") {
		t.Fatalf("markdown was not rebuilt from the solved HTML: %q", result.Markdown)
	}
	if solver.got.Vendor != "cloudflare" {
		t.Fatalf("solver received vendor %q, want cloudflare", solver.got.Vendor)
	}
	if solver.got.URL != "https://example.com" {
		t.Fatalf("solver received URL %q", solver.got.URL)
	}
}

func TestService_SolverFailureStillReportsBlock(t *testing.T) {
	solver := &fakeSolver{err: errors.New("addon is down")}
	svc := NewService(&mockScraper{result: challengeResult(t)}, nil, nil, WithSolver(solver))

	_, err := svc.Scrape(context.Background(), "https://example.com", "static", domain.ScrapeOptions{})
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("error = %T %v, want *BlockedError", err, err)
	}
}

func TestService_BlockedStatusCarriesChallengeBody(t *testing.T) {
	// A 403 whose body is a challenge page must reach the solver with the body
	// intact, not be flattened into a plain status error.
	body := fixture(t, "cloudflare_challenge.html")
	static := &mockScraper{err: &BlockedError{
		URL: "https://example.com", Vendor: "cloudflare", StatusCode: 403, HTML: body,
	}}
	solver := &fakeSolver{solution: &domain.ChallengeSolution{
		HTML: "<html><body><h1>Solved after 403</h1>" + makeStaticHTML() + "</body></html>",
	}}
	svc := NewService(static, nil, nil, WithSolver(solver))

	result, err := svc.Scrape(context.Background(), "https://example.com", "static", domain.ScrapeOptions{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if solver.got.HTML != body {
		t.Fatal("solver did not receive the challenge body")
	}
	if !strings.Contains(result.Markdown, "Solved after 403") {
		t.Fatalf("markdown = %q", result.Markdown)
	}
}
