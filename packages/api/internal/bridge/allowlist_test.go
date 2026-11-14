package bridge

import (
	"path/filepath"
	"testing"
)

// TestNormalizeHostPattern: the allowlist stores exactly one shape, so every
// way a person might type a host has to land on it. A pattern that survives
// normalization but matches nothing is a silent hole.
func TestNormalizeHostPattern(t *testing.T) {
	cases := []struct{ in, want string }{
		{"google.com", "google.com"},
		{"  Google.COM  ", "google.com"},
		{"https://www.google.com/search?q=x", "www.google.com"},
		{"http://example.com:8080/path", "example.com"},
		{"*.example.com", "*.example.com"},
		{"https://*.example.com/x", "*.example.com"},
		{"", ""},
		{"https://", ""},
		{"*", ""},
		{"a..b", ""},
	}
	for _, tc := range cases {
		if got := NormalizeHostPattern(tc.in); got != tc.want {
			t.Errorf("NormalizeHostPattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestMatchesHostPatterns locks the wildcard semantics: `*.example.com` means
// the domain and its subdomains, because that is what a person means by it.
func TestMatchesHostPatterns(t *testing.T) {
	cases := []struct {
		patterns []string
		host     string
		want     bool
	}{
		{[]string{"example.com"}, "example.com", true},
		{[]string{"example.com"}, "www.example.com", false},
		{[]string{"*.example.com"}, "example.com", true},
		{[]string{"*.example.com"}, "docs.example.com", true},
		{[]string{"*.example.com"}, "notexample.com", false},
		{[]string{"*.example.com"}, "example.com.evil.test", false},
		{nil, "example.com", false},
		{[]string{"", "example.com"}, "example.com", true},
	}
	for _, tc := range cases {
		if got := MatchesHostPatterns(tc.patterns, tc.host); got != tc.want {
			t.Errorf("MatchesHostPatterns(%v, %q) = %v, want %v", tc.patterns, tc.host, got, tc.want)
		}
	}
}

// TestHostOfOrigin handles the three shapes a caller actually has: an origin, a
// URL and a bare host.
func TestHostOfOrigin(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://google.com", "google.com"},
		{"https://www.google.com/search?q=svelte", "www.google.com"},
		{"localhost:7445", "localhost"},
		{"example.com", "example.com"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := HostOfOrigin(tc.in); got != tc.want {
			t.Errorf("HostOfOrigin(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestAllowlistRoundTrip drives the real State: allow, deny, and the fact that a
// fresh state arrives with the search engines already allowed.
//
// The default matters: the browser search rung is the reason the lane exists, so
// a new install that had to allow duckduckgo.com by hand before its first search
// would be a bad first hour.
func TestAllowlistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := NewState(dir, nil)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}

	if !st.Allows("https://html.duckduckgo.com/html?q=x") {
		t.Fatalf("a fresh state should allow the DuckDuckGo HTML endpoint")
	}
	if st.Allows("https://mail.google.com") {
		t.Fatalf("a fresh state must NOT allow mail.google.com: the default is the search hosts, not the whole domain")
	}

	added, err := st.Allow("router.home")
	if err != nil || !added {
		t.Fatalf("Allow(router.home) = %v, %v; want true, nil", added, err)
	}
	if again, err := st.Allow("https://router.home/admin"); err != nil || again {
		t.Fatalf("allowing the same host twice should report false, got %v, %v", again, err)
	}
	if !st.Allows("http://router.home/x") {
		t.Fatalf("the entry just added should match")
	}

	removed, err := st.Deny("router.home")
	if err != nil || !removed {
		t.Fatalf("Deny = %v, %v; want true, nil", removed, err)
	}
	if st.Allows("http://router.home/x") {
		t.Fatalf("a denied host must not match")
	}

	if _, err := st.Allow("not a host"); err == nil {
		t.Fatalf("a bogus pattern must be refused, not stored")
	}

	// The file is the contract: a CLI edit and a server running elsewhere have to
	// agree without a restart.
	if err := st.saveLocked(); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := ReadInfo(dir)
	if err != nil {
		t.Fatalf("ReadInfo: %v", err)
	}
	found := false
	for _, o := range info.AllowedOrigins {
		if o == "duckduckgo.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("persisted allowlist should still hold the defaults, got %v", info.AllowedOrigins)
	}
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatalf("temp dir: %v", err)
	}
}

// TestAllowsReloadsExternalEdit: editing bridge.json is the documented way to
// change the allowlist by hand, so a running server must notice.
func TestAllowsReloadsExternalEdit(t *testing.T) {
	dir := t.TempDir()
	st, err := NewState(dir, nil)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if st.Allows("https://internal.example") {
		t.Fatalf("internal.example should not be allowed yet")
	}

	other, err := NewState(dir, nil)
	if err != nil {
		t.Fatalf("second NewState: %v", err)
	}
	if _, err := other.Allow("internal.example"); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	if !st.Allows("https://internal.example") {
		t.Fatalf("the first State should have reloaded the file written by the second")
	}
}
