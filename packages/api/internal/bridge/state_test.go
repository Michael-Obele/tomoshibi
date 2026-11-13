package bridge

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestStateCodeAndPairPersistence(t *testing.T) {
	dir := t.TempDir()
	st, err := NewState(dir, nil)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if !regexp.MustCompile(`^\d{6}$`).MatchString(st.Code()) {
		t.Fatalf("code %q is not six digits", st.Code())
	}

	code := st.Code()
	if _, _, err := st.Pair("999999", "test"); err == nil {
		t.Fatal("wrong code must not pair")
	}
	tok, serverKey, err := st.Pair(code, "test")
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if serverKey == "" || st.ServerKey() != serverKey {
		t.Fatal("pairing must yield a stable server key")
	}
	if !st.ValidToken(tok) {
		t.Fatal("fresh token must validate")
	}
	if st.ValidToken("nope") {
		t.Fatal("bogus token must not validate")
	}
	if st.Code() == code {
		t.Fatal("code must rotate after a successful pair")
	}

	// The token hash survives a restart; the plaintext exists only in the reply.
	st2, err := NewState(dir, nil)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !st2.ValidToken(tok) {
		t.Fatal("paired device must survive a state reload")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "bridge.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if stringContains(string(raw), tok) {
		t.Fatal("plaintext token must never be written to disk")
	}
}

func stringContains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
