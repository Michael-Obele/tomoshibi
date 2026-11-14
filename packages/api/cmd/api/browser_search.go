package main

import (
	"context"
	"fmt"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/Michael-Obele/tomoshibi/internal/search"
)

// browserSearchRung builds the browser-backed search engine (D32). It reads
// DuckDuckGo by default; one request may name google or bing instead.
func browserSearchRung(tr *scraper.BridgeTransport) search.Service {
	if tr == nil {
		return nil
	}
	return search.NewBrowserService(bridgeFetcher{tr: tr}, search.BrowserDDG)
}

// bridgeFetcher reads a URL through the user's own signed-in browser, which is
// what makes the engines that block a datacenter address answer again.
//
// It lives here, not in internal/search, so that package keeps its rule of
// importing nothing from the project: search defines the BrowserFetcher seam,
// and the composition root supplies what it needs.
//
// It uses FetchRaw rather than a full scrape on purpose: the SERP parser decides
// what a challenge is (parse first, refuse only when nothing parsed), which is
// more precise than the generic content detector.
type bridgeFetcher struct{ tr *scraper.BridgeTransport }

func (f bridgeFetcher) FetchHTML(ctx context.Context, rawURL string) (string, error) {
	return f.fetch(ctx, rawURL, nil)
}

// FetchHTMLWithActions drives the page before reading it. The only caller is the
// consent-wall retry: a wall cannot be read past, only answered, and answering it
// in the browser that holds the cookie is the whole point of this lane.
func (f bridgeFetcher) FetchHTMLWithActions(ctx context.Context, rawURL string, actions []search.FetchAction) (string, error) {
	converted := make([]domain.Action, 0, len(actions))
	for _, a := range actions {
		converted = append(converted, domain.Action{
			Type: a.Type, Selector: a.Selector, Script: a.Script, Ms: a.Ms,
		})
	}
	return f.fetch(ctx, rawURL, converted)
}

func (f bridgeFetcher) fetch(ctx context.Context, rawURL string, actions []domain.Action) (string, error) {
	html, err := f.tr.FetchRaw(ctx, rawURL, domain.ScrapeOptions{Actions: actions})
	if err != nil {
		return "", err
	}
	if html == "" {
		return "", fmt.Errorf("browser returned no HTML for %s", rawURL)
	}
	return html, nil
}
