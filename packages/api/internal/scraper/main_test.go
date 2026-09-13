package scraper

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// TestMain initializes the shared logger so real scraper callbacks
// (e.g. colly OnRequest) don't panic on a nil logger.Log.
//
// It also opts out of safeurl's private-address blocking: httptest servers
// bind to 127.0.0.1, which the SSRF guard refuses by design. Tests that
// exercise the guard itself set the variable explicitly with t.Setenv.
func TestMain(m *testing.M) {
	logger.Init("error")
	os.Setenv(safeurl.AllowPrivateEnv, "true")
	os.Exit(m.Run())
}

// TestSSRFGuardWired asserts the colly scraper actually fetches through
// safeurl.Transport. The rest of this package runs with the guard disabled
// (see TestMain), so without this a regression that dropped the WithTransport
// call would pass CI in silence. This is the primary scrape path, so it is
// the one that matters most.
func TestSSRFGuardWired(t *testing.T) {
	t.Setenv(safeurl.AllowPrivateEnv, "false")

	// The cloud metadata endpoint. Nothing needs to be listening: the guard
	// rejects the address before a connection is attempted.
	_, err := NewCollyScraper().Scrape(
		context.Background(),
		"http://169.254.169.254/latest/meta-data/",
		domain.ScrapeOptions{},
	)
	if err == nil {
		t.Fatal("expected the scrape to be blocked, got nil error")
	}
	if !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected an SSRF block, got: %v", err)
	}
}
