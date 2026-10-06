package envctx

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestGetPrefersRequestOverride(t *testing.T) {
	t.Setenv("TOMOSHIBI_TEST_ENVCTX_A", "from-process")
	ctx := With(context.Background(), Overrides{"TOMOSHIBI_TEST_ENVCTX_A": "from-request"})

	if got := Get(ctx, "TOMOSHIBI_TEST_ENVCTX_A"); got != "from-request" {
		t.Errorf("Get = %q, want request override", got)
	}
	// Names not overridden still resolve from the process env.
	if got := Get(ctx, "TOMOSHIBI_TEST_ENVCTX_A_MISSING"); got != "" {
		t.Errorf("Get(unset) = %q, want empty", got)
	}
	// A background context behaves exactly like os.Getenv.
	if got := Get(context.Background(), "TOMOSHIBI_TEST_ENVCTX_A"); got != "from-process" {
		t.Errorf("Get(bg) = %q, want process env", got)
	}
}

func TestExpandUsesOverrides(t *testing.T) {
	t.Setenv("TOMOSHIBI_TEST_ENVCTX_B", "process-value")
	ctx := With(context.Background(), Overrides{"TOMOSHIBI_TEST_ENVCTX_B": "request-value"})

	if got := Expand(ctx, "k=${TOMOSHIBI_TEST_ENVCTX_B}"); got != "k=request-value" {
		t.Errorf("Expand = %q, want override applied", got)
	}
	if got := Expand(ctx, "k=${TOMOSHIBI_TEST_ENVCTX_B_MISSING}"); got != "k=" {
		t.Errorf("Expand(missing) = %q, want empty substitution", got)
	}
	// No "$" = fast path returns the input unchanged.
	if got := Expand(ctx, "plain"); got != "plain" {
		t.Errorf("Expand(plain) = %q", got)
	}
}

func TestWithEmptyOverridesIsNoop(t *testing.T) {
	parent := context.Background()
	if got := With(parent, nil); got != parent {
		t.Error("With(nil overrides) should return the parent context unchanged")
	}
}

func TestParseKeepsOnlyAllowedNames(t *testing.T) {
	ov, err := Parse(`{"BRAVE_SEARCH_API_KEY":"BSB-test","REDIS_URL":"redis://evil","random":"x"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(ov) != 1 || ov["BRAVE_SEARCH_API_KEY"] != "BSB-test" {
		t.Fatalf("ov = %#v, want only the allowed name", ov)
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	tooBig := make([]byte, maxHeaderBytes+1)
	for i := range tooBig {
		tooBig[i] = 'a'
	}
	many := `{"KEEP":"1"`
	for i := 0; i < maxEntries; i++ {
		many += fmt.Sprintf(`,"X%d":"1"`, i)
	}
	many += "}"

	cases := []struct {
		name string
		raw  string
	}{
		{"not json", "BRAVE_SEARCH_API_KEY=x"},
		{"not an object", `["BRAVE_SEARCH_API_KEY"]`},
		{"non-string value", `{"BRAVE_SEARCH_API_KEY":42}`},
		{"too many entries", many},
		{"oversized header", string(tooBig)},
		{"oversized value", `{"BRAVE_SEARCH_API_KEY":"` + strings.Repeat("v", maxValueLen+1) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.raw); err == nil {
				t.Fatal("Parse accepted malformed input")
			}
		})
	}
}

func TestParseDropsBlankAndJunkValues(t *testing.T) {
	ov, err := Parse(`{"BRAVE_SEARCH_API_KEY":"   ","TOMOSHI_API_KEY":"sk-x"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(ov) != 0 {
		t.Errorf("ov = %#v, want empty (blank value and non-allowed name dropped)", ov)
	}
}

func TestAllowedIsAllowlistPolicy(t *testing.T) {
	names := Allowed()
	if len(names) == 0 {
		t.Fatal("Allowed() must not be empty")
	}
	for _, name := range names {
		if !namePattern.MatchString(name) {
			t.Errorf("Allowed() entry %q does not match the env name pattern", name)
		}
	}
	// One allowlist entry per keyed engine in the roster; all must parse as
	// env names so MCP forwarding and header validation agree.
	for _, want := range []string{"BRAVE_SEARCH_API_KEY", "SERPER_API_KEY", "TAVILY_API_KEY"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Allowed() missing %s", want)
		}
	}
	// Infra credentials must never join the allowlist.
	for _, forbidden := range []string{"REDIS_URL", "WEBSHARE_API_KEY", "APP_API_KEYS"} {
		for _, name := range names {
			if name == forbidden {
				t.Errorf("%s must not be client-suppliable", forbidden)
			}
		}
	}
}
