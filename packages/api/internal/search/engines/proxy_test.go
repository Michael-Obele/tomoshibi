package engines

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyPoolRotationAndFallback(t *testing.T) {
	p := NewProxyPool(ProxyOptions{WebshareProxyURL: "http://a:1, http://b:2"})
	var got []string
	for i := 0; i < 3; i++ {
		got = append(got, p.Next("webshare"))
	}
	want := []string{"http://a:1", "http://b:2", "http://a:1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("rotation = %v, want %v", got, want)
	}
	if u := p.Next("tier1"); u != "" {
		t.Errorf("unconfigured group must degrade to direct, got %q", u)
	}
	if u := p.Next(""); u != "" {
		t.Errorf("empty group must be direct, got %q", u)
	}
}

func TestProxyPoolBudget(t *testing.T) {
	// 0.0000001 GB ≈ 107 bytes — one spend blows the cap.
	p := NewProxyPool(ProxyOptions{WebshareProxyURL: "http://a:1", BudgetGB: "0.0000001"})
	if u := p.Next("webshare"); u == "" {
		t.Fatal("first use must be allowed")
	}
	p.Spend(1 << 20)
	if u := p.Next("webshare"); u != "" {
		t.Errorf("budget exhausted but proxy still served: %q", u)
	}
	if !p.BudgetExhausted() {
		t.Error("BudgetExhausted should be true")
	}

	// Explicit 0 = unlimited opt-out.
	p2 := NewProxyPool(ProxyOptions{WebshareProxyURL: "http://a:1", BudgetGB: "0"})
	p2.Spend(1 << 40)
	if u := p2.Next("webshare"); u == "" {
		t.Error("BudgetGB=0 must mean unlimited")
	}
}

func TestProxyPoolWebshareAPIList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/proxy/list/" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Token test-key-123" {
			t.Errorf("auth header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":3,"results":[
			{"username":"u","password":"p","proxy_address":"1.2.3.4","port":8168,"valid":true},
			{"username":"u","password":"p","proxy_address":"5.6.7.8","port":8168,"valid":true},
			{"username":"u","password":"p","proxy_address":"9.9.9.9","port":8168,"valid":false}
		]}`))
	}))
	defer srv.Close()

	p := NewProxyPool(ProxyOptions{WebshareAPIKey: "test-key-123"})
	p.webshareAPIBase = srv.URL

	first := p.Next("webshare")
	if first != "http://u:p@1.2.3.4:8168" {
		t.Fatalf("first = %q", first)
	}
	if second := p.Next("webshare"); second != "http://u:p@5.6.7.8:8168" {
		t.Fatalf("second = %q (invalid entries must be skipped, round-robin over valid)", second)
	}
	if third := p.Next("webshare"); third != first {
		t.Errorf("rotation should wrap: %q", third)
	}
}

func TestProxyPoolWebshareAPIFailureDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := NewProxyPool(ProxyOptions{WebshareAPIKey: "bad"})
	p.webshareAPIBase = srv.URL
	if u := p.Next("webshare"); u != "" {
		t.Errorf("API failure must degrade to direct, got %q", u)
	}
	// Second call: sync.Once already fired — still direct, no panic.
	if u := p.Next("webshare"); u != "" {
		t.Errorf("still direct, got %q", u)
	}
}

func TestProxyPoolTierGroups(t *testing.T) {
	p := NewProxyPool(ProxyOptions{Tier1: "http://t1:1", Tier2: "http://t2a:1,http://t2b:2"})
	if u := p.Next("tier1"); u != "http://t1:1" {
		t.Errorf("tier1 = %q", u)
	}
	if u := p.Next("tier2"); u != "http://t2a:1" {
		t.Errorf("tier2 = %q", u)
	}
	if u := p.Next("tier2"); u != "http://t2b:2" {
		t.Errorf("tier2 rotation = %q", u)
	}
}

// TestProxyPoolWebsharePagination follows the docs' `next` field across
// pages so round-robin covers the whole pool, not just page 1.
func TestProxyPoolWebsharePagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "", "1":
			_, _ = w.Write([]byte(`{"count":2,"next":"/api/v2/proxy/list/?mode=direct&page=2&page_size=100","previous":null,"results":[
				{"username":"u","password":"p","proxy_address":"1.2.3.4","port":8168,"valid":true}
			]}`))
		case "2":
			_, _ = w.Write([]byte(`{"count":2,"next":null,"previous":"x","results":[
				{"username":"u","password":"p","proxy_address":"5.6.7.8","port":8168,"valid":true}
			]}`))
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	p := NewProxyPool(ProxyOptions{WebshareAPIKey: "k"})
	p.webshareAPIBase = srv.URL

	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		u := p.Next("webshare")
		if u == "" {
			t.Fatalf("empty proxy at %d", i)
		}
		seen[u] = true
	}
	if len(seen) != 2 {
		t.Errorf("round-robin should cover both pages, saw %v", seen)
	}
}
