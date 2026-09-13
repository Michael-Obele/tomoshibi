package image

import (
	"os"
	"strings"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
)

// TestSSRFGuardWired asserts the processor actually fetches through
// safeurl.Client. Everything else in this package runs with the guard
// disabled (see TestMain), so without this test a regression that swapped the
// client back to http.DefaultClient would pass CI in silence.
//
// The guard is re-enabled for the duration of this test only.
func TestSSRFGuardWired(t *testing.T) {
	// TestMain sets this process-wide; t.Setenv restores it on cleanup.
	t.Setenv(safeurl.AllowPrivateEnv, "false")

	// 169.254.169.254 is the cloud metadata endpoint — the exact target an
	// attacker aims a hostile <img src> at. No server needs to be listening:
	// the guard rejects the address before a connection is attempted.
	_, err := NewProcessor().FetchAndEncode("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected the fetch to be blocked, got nil error")
	}
	if !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected an SSRF block, got: %v", err)
	}
}

// TestMain opts out of safeurl's private-address blocking. Processor fetches
// through a guarded http.Client, and the httptest servers these tests spin up
// bind to 127.0.0.1, which the guard refuses by design.
func TestMain(m *testing.M) {
	os.Setenv(safeurl.AllowPrivateEnv, "true")
	os.Exit(m.Run())
}
