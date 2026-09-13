package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// TestValidateCategory verifies the enum validation for the Category field.
func TestValidateCategory(t *testing.T) {
	tests := []struct {
		name    string
		cat     string
		wantErr bool
	}{
		{"empty allowed", "", false},
		{"general allowed", "general", false},
		{"news allowed", "news", false},
		{"code allowed", "code", false},
		{"invalid company", "company", true},
		{"invalid people", "people", true},
		{"invalid empty string with spaces", " General", true},
		{"invalid random", "invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCategory(tt.cat)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateCategory(%q) expected error, got nil", tt.cat)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateCategory(%q) unexpected error: %v", tt.cat, err)
			}
		})
	}
}

// TestSearXNGCategoryMapping verifies Category maps to SearXNG categories param.
func TestSearXNGCategoryMapping(t *testing.T) {
	tests := []struct {
		name     string
		category string
		wantCat  string // expected categories param value; "" means absent
	}{
		{"general maps to general", "general", "general"},
		{"news maps to news", "news", "news"},
		{"code maps to it", "code", "it"},
		{"empty omits categories", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotCategories string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotCategories = r.URL.Query().Get("categories")
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(searxngResponse{
					Query: "q",
					Results: []searxngResult{
						{URL: "https://example.com/", Title: "Example", Content: "desc"},
					},
				})
			}))
			defer srv.Close()
			svc := NewSearXNGService(srv.URL)
			_, _, err := svc.Search(context.Background(), SearchOptions{Query: "q", Category: tt.category, Limit: 10})
			if err != nil {
				t.Fatalf("Search returned error: %v", err)
			}
			if gotCategories != tt.wantCat {
				t.Errorf("categories = %q, want %q", gotCategories, tt.wantCat)
			}
		})
	}
}

// TestSearXNGCategoryOverridesMode verifies Category takes precedence over legacy Mode.
func TestSearXNGCategoryOverridesMode(t *testing.T) {
	var gotCategories string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCategories = r.URL.Query().Get("categories")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searxngResponse{Query: "q", Results: []searxngResult{{URL: "https://a.dev/", Title: "A"}}})
	}))
	defer srv.Close()
	svc := NewSearXNGService(srv.URL)
	// Category=general should win over Mode=news
	if _, _, err := svc.Search(context.Background(), SearchOptions{Query: "q", Category: "general", Mode: "news"}); err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if gotCategories != "general" {
		t.Errorf("expected general to override mode news, got %q", gotCategories)
	}
}

// TestSearXNGCategoryInvalid verifies invalid category returns error and no request is made.
func TestSearXNGCategoryInvalid(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	svc := NewSearXNGService(srv.URL)
	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "q", Category: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid category, got nil")
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("error = %v, want it to mention invalid category", err)
	}
	if called {
		t.Error("SearXNG upstream was contacted despite invalid category")
	}
}

// TestSearXNGModeFallback verifies legacy Mode still maps when Category is empty.
func TestSearXNGModeFallback(t *testing.T) {
	tests := []struct {
		mode    string
		wantCat string
	}{
		{"news", "news"},
		{"code", "it"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query().Get("categories")
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(searxngResponse{Results: []searxngResult{{URL: "https://a.dev/", Title: "A"}}})
			}))
			defer srv.Close()
			svc := NewSearXNGService(srv.URL)
			if _, _, err := svc.Search(context.Background(), SearchOptions{Query: "q", Mode: tt.mode}); err != nil {
				t.Fatalf("Search error: %v", err)
			}
			if got != tt.wantCat {
				t.Errorf("mode %q mapped to categories %q, want %q", tt.mode, got, tt.wantCat)
			}
		})
	}
}

// TestBraveCategoryMapping verifies Category maps to Brave search_type.
func TestBraveCategoryMapping(t *testing.T) {
	tests := []struct {
		name     string
		category string
		wantType string
	}{
		{"general maps to general", "general", "general"},
		{"news maps to news", "news", "news"},
		{"code maps to code", "code", "code"},
		{"empty omits search_type", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw string
			svc := newTestService(t, respondWith(1, &raw))
			// Ensure limiter doesn't block
			svc.limiter = rate.NewLimiter(rate.Inf, 1)
			if _, _, err := svc.Search(context.Background(), SearchOptions{Query: "q", Category: tt.category}); err != nil {
				t.Fatalf("Search error: %v", err)
			}
			q, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			if got := q.Get("search_type"); got != tt.wantType {
				t.Errorf("search_type = %q, want %q (raw=%s)", got, tt.wantType, raw)
			}
		})
	}
}

// TestBraveCategoryInvalid verifies invalid category returns error before request.
func TestBraveCategoryInvalid(t *testing.T) {
	called := false
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	svc.limiter = rate.NewLimiter(rate.Inf, 1)
	_, _, err := svc.Search(context.Background(), SearchOptions{Query: "q", Category: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid category, got nil")
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("error = %v, want invalid category", err)
	}
	if called {
		t.Error("Brave upstream contacted despite invalid category")
	}
}

// TestSearchCacheKeyIncludesCategory verifies distinct categories produce distinct keys
// and identical categories produce identical keys.
func TestSearchCacheKeyIncludesCategory(t *testing.T) {
	a := SearchOptions{Query: "golang", Limit: 10, Category: "general"}
	b := SearchOptions{Query: "golang", Limit: 10, Category: "general"}
	c := SearchOptions{Query: "golang", Limit: 10, Category: "news"}
	d := SearchOptions{Query: "golang", Limit: 10, Category: "code"}
	e := SearchOptions{Query: "golang", Limit: 10, Category: ""}
	if searchCacheKey(a) != searchCacheKey(b) {
		t.Error("identical category general must produce identical cache keys")
	}
	if searchCacheKey(a) == searchCacheKey(c) {
		t.Error("general vs news must produce different cache keys")
	}
	if searchCacheKey(c) == searchCacheKey(d) {
		t.Error("news vs code must produce different cache keys")
	}
	if searchCacheKey(a) == searchCacheKey(e) {
		t.Error("general vs empty must produce different cache keys")
	}
	// Also verify Mode still part of key (distinct from category)
	f := SearchOptions{Query: "golang", Limit: 10, Mode: "fast"}
	g := SearchOptions{Query: "golang", Limit: 10, Mode: "fast", Category: "news"}
	if searchCacheKey(f) == searchCacheKey(g) {
		t.Error("mode+category combination must produce different key than mode alone")
	}
}
