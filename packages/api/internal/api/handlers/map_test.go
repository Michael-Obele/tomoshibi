package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestMapEndpoint_SitemapDiscovery exercises POST /v1/map end-to-end.
func TestMapEndpoint_SitemapDiscovery(t *testing.T) {
	var site *httptest.Server
	site = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprintf(w, "Sitemap: %s/sitemap.xml\n", site.URL)
		case "/sitemap.xml":
			fmt.Fprintf(w, `<urlset><url><loc>%s/blog/a</loc></url><url><loc>%s/blog/b</loc></url><url><loc>%s/about</loc></url></urlset>`, site.URL, site.URL, site.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()

	router := gin.New()
	router.POST("/v1/map", NewMapHandler().Map)

	body, _ := json.Marshal(map[string]any{"url": site.URL})
	req := httptest.NewRequest(http.MethodPost, "/v1/map", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp MapResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 3 {
		t.Errorf("expected 3 URLs, got %d: %+v", resp.Count, resp.Links)
	}
}

// TestMapEndpoint_SearchFilter verifies the search substring filter.
func TestMapEndpoint_SearchFilter(t *testing.T) {
	var site *httptest.Server
	site = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprintf(w, "Sitemap: %s/sitemap.xml\n", site.URL)
		case "/sitemap.xml":
			fmt.Fprintf(w, `<urlset><url><loc>%s/blog/a</loc></url><url><loc>%s/docs/x</loc></url></urlset>`, site.URL, site.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()

	router := gin.New()
	router.POST("/v1/map", NewMapHandler().Map)

	body, _ := json.Marshal(map[string]any{"url": site.URL, "search": "blog"})
	req := httptest.NewRequest(http.MethodPost, "/v1/map", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp MapResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 || len(resp.Links) != 1 {
		t.Errorf("expected 1 filtered URL, got %d", resp.Count)
	}
}
