package solver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestHTTP_Solve(t *testing.T) {
	var gotAuth string
	var gotReq domain.ChallengeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(domain.ChallengeSolution{HTML: "<html><body>solved</body></html>"})
	}))
	defer srv.Close()

	client := NewHTTP(srv.URL, "secret")
	solution, err := client.Solve(context.Background(), domain.ChallengeRequest{
		URL:    "https://example.com",
		Vendor: "cloudflare",
		HTML:   "<html>challenge</html>",
	})
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if solution.HTML != "<html><body>solved</body></html>" {
		t.Fatalf("HTML = %q", solution.HTML)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q, want Bearer secret", gotAuth)
	}
	if gotReq.URL != "https://example.com" || gotReq.Vendor != "cloudflare" {
		t.Fatalf("solver received %+v", gotReq)
	}
}

func TestHTTP_SolveErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "non-2xx",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", http.StatusBadGateway)
			},
		},
		{
			name: "empty solution",
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(domain.ChallengeSolution{})
			},
		},
		{
			name: "invalid json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("not json"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			if _, err := NewHTTP(srv.URL, "").Solve(context.Background(), domain.ChallengeRequest{}); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestHTTP_SolveRespectsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := NewHTTP(srv.URL, "").Solve(ctx, domain.ChallengeRequest{}); err == nil {
		t.Fatal("expected a cancellation error, got nil")
	}
}
