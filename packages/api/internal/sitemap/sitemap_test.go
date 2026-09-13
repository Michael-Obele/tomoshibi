package sitemap

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDiscover_FromRobotsAndSitemapIndex covers robots.txt → sitemap index
// → child sitemap → page URLs.
func TestDiscover_FromRobotsAndSitemapIndex(t *testing.T) {
	var hits []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprintf(w, "User-agent: *\nDisallow: /admin\nSitemap: %s/sitemap-index.xml\n", srv.URL)
		case "/sitemap-index.xml":
			fmt.Fprintf(w, `<sitemapindex><sitemap><loc>%s/sitemap-1.xml</loc></sitemap></sitemapindex>`, srv.URL)
		case "/sitemap-1.xml":
			fmt.Fprintf(w, `<urlset><url><loc>%s/page1</loc></url><url><loc>%s/page2</loc></url></urlset>`, srv.URL, srv.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	found, err := Discover(context.Background(), srv.URL, 100)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %+v", len(found), found)
	}
	for _, u := range found {
		if u.Source != "sitemap" {
			t.Errorf("expected sitemap source, got %q", u.Source)
		}
	}
}

// TestDiscover_LinkFallback covers sites with no sitemap: one-level link
// discovery from the seed page.
func TestDiscover_LinkFallback(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r) // no robots
		case "/sitemap.xml":
			http.NotFound(w, r) // no sitemap
		default:
			fmt.Fprintf(w, `<html><body><a href="%s/a">A</a><a href="%s/b">B</a><a href="https://external.com/x">X</a></body></html>`, srv.URL, srv.URL)
		}
	}))
	defer srv.Close()

	found, err := Discover(context.Background(), srv.URL, 100)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 fallback links, got %d: %+v", len(found), found)
	}
	for _, u := range found {
		if u.Source != "link" {
			t.Errorf("expected link source, got %q", u.Source)
		}
		if strings.Contains(u.URL, "external.com") {
			t.Errorf("external link must be filtered: %q", u.URL)
		}
	}
}

func TestParseRobotsSitemaps(t *testing.T) {
	robots := "User-agent: *\nDisallow: /admin\nSitemap: https://x.com/sitemap.xml\n\nSitemap: https://x.com/sitemap2.xml\n"
	got := parseRobotsSitemaps([]byte(robots))
	if len(got) != 2 || got[0] != "https://x.com/sitemap.xml" || got[1] != "https://x.com/sitemap2.xml" {
		t.Errorf("unexpected sitemap lines: %v", got)
	}
}
