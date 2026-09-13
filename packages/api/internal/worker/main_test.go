package worker

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

// TestMain initializes the shared logger (worker handlers log through it, and
// a nil logger.Log panics) and opts out of safeurl's private-address
// blocking: webhook delivery goes through a guarded http.Client and the
// httptest servers these tests spin up bind to 127.0.0.1.
//
// It also zeroes the per-domain politeness delay. Every crawl test points at
// a single httptest host, so the production 1s default serialized the whole
// suite behind it — 11s for one wide-page test alone — while testing nothing:
// these cases cover BFS traversal, depth, limits and deadlock, not politeness.
// A test that does exercise the delay should set it explicitly with t.Setenv,
// which works because the crawl tuning helpers read the environment at call
// time rather than caching it.
func TestMain(m *testing.M) {
	logger.Init("error")
	os.Setenv(safeurl.AllowPrivateEnv, "true")
	os.Setenv("CRAWL_DOMAIN_DELAY", "0")

	// Collapse the retry backoff. The retry tests assert how many attempts
	// were made, never how long they took, so the production 1s/2s waits were
	// dead time — the webhook and monitor cases spent 5s each purely asleep.
	retryBackoffUnit = time.Millisecond

	os.Exit(m.Run())
}

// TestSSRFGuardWired asserts Deliver actually posts through safeurl.Client.
// The rest of this package runs with the guard disabled (see TestMain), so
// without this a regression back to http.DefaultClient would pass CI in
// silence. webhook_url is caller-supplied, which is what makes it reachable.
func TestSSRFGuardWired(t *testing.T) {
	t.Setenv(safeurl.AllowPrivateEnv, "false")

	// The cloud metadata endpoint. Nothing needs to be listening: the guard
	// rejects the address before a connection is attempted.
	err := Deliver(context.Background(), "http://169.254.169.254/latest/meta-data/", "", []byte(`{}`))
	if err == nil {
		t.Fatal("expected the webhook delivery to be blocked, got nil error")
	}
	if !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected an SSRF block, got: %v", err)
	}
}
