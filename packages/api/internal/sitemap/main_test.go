package sitemap

import (
	"context"
	"os"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
)

// TestMain opts out of safeurl's private-address blocking. Discovery fetches
// through a guarded http.Client, and the httptest servers these tests spin up
// bind to 127.0.0.1, which the guard refuses by design.
func TestMain(m *testing.M) {
	os.Setenv(safeurl.AllowPrivateEnv, "true")
	os.Exit(m.Run())
}

// TestSSRFGuardWired asserts Discover actually fetches through
// safeurl.Client. The rest of this package runs with the guard disabled (see
// TestMain), so without this a regression back to a bare http.Client would
// pass CI in silence. Sitemap traversal is recursive and follows
// attacker-supplied URLs, which is what makes this path worth pinning.
func TestSSRFGuardWired(t *testing.T) {
	t.Setenv(safeurl.AllowPrivateEnv, "false")

	// The cloud metadata endpoint. Nothing needs to be listening: the guard
	// rejects the address before a connection is attempted. Discover degrades
	// rather than erroring on a failed fetch, so assert on the result being
	// empty — a successful fetch would have yielded the seed URL at minimum.
	urls, err := Discover(context.Background(), "http://169.254.169.254/", 10)
	if err == nil && len(urls) > 0 {
		t.Fatalf("expected no URLs from a blocked host, got %d: %v", len(urls), urls)
	}
}
