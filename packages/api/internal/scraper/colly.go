package scraper

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/fingerprint"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
	"github.com/gocolly/colly/v2"
)

type CollyScraper struct {
	transport    http.RoundTripper
	transportErr error
}

// NewCollyScraper builds the static engine with a browser-grade transport:
// Chrome's TLS fingerprint and a User-Agent that matches it. Set
// SCRAPE_TLS_PROFILE=go to fall back to Go's own TLS stack, which is faster to
// start but trivially distinguishable from a browser.
func NewCollyScraper() *CollyScraper {
	if os.Getenv("SCRAPE_TLS_PROFILE") == "go" {
		return &CollyScraper{transport: safeurl.Transport()}
	}
	rt, err := fingerprint.Transport()
	if err != nil {
		// Do not silently degrade to the Go stack: a request that claims to be
		// Chrome while handshaking like Go is worse than failing loudly.
		return &CollyScraper{transportErr: fmt.Errorf("browser TLS transport: %w", err)}
	}
	return &CollyScraper{transport: rt}
}

func (s *CollyScraper) Scrape(ctx context.Context, url string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.transportErr != nil {
		return nil, s.transportErr
	}

	c := colly.NewCollector(
		colly.Async(true),
	)

	// The fingerprint transport dials through safeurl, so redirects and DNS
	// rebinding are still checked at connection time.
	c.WithTransport(s.transport)

	c.OnRequest(func(r *colly.Request) {
		logger.Log.Info("Scraping", "url", r.URL)
	})

	timeout := 30 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d < timeout {
			timeout = d
		}
	}
	c.SetRequestTimeout(timeout)

	var htmlContent string
	var scrapeErr error

	c.OnHTML("html", func(e *colly.HTMLElement) {
		htmlContent, _ = e.DOM.Html()
	})

	c.OnError(func(r *colly.Response, err error) {
		if r == nil {
			scrapeErr = fmt.Errorf("scraping failed: %w", err)
			return
		}
		if r.StatusCode >= 400 {
			if ch := detectChallenge(r.Body, r.StatusCode); ch != nil {
				// A 403 that is really an anti-bot wall must not be reported as
				// a plain upstream status: the caller needs to know the body is
				// not content.
				scrapeErr = &BlockedError{
					URL:        requestURL(r),
					Vendor:     ch.Vendor,
					StatusCode: r.StatusCode,
					HTML:       challengeBody(r.Body),
					Err:        err,
				}
				return
			}
			// Carry the HTTP status so retry policies can skip 4xx and the
			// throttle can honour a Retry-After the origin actually sent.
			scrapeErr = &StatusError{
				StatusCode: r.StatusCode,
				RetryAfter: parseRetryAfter(r.Headers.Get("Retry-After")),
				Err:        err,
			}
			return
		}
		scrapeErr = fmt.Errorf("scraping failed: %w", err)
	})

	err := c.Visit(url)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	}

	if scrapeErr != nil {
		return nil, scrapeErr
	}

	if htmlContent == "" {
		return nil, fmt.Errorf("empty response")
	}

	// Apply cleaner-output defaults (block ads, drop base64 images) before
	// readability strips the class/id/aria-label attributes the selectors need.
	return buildStaticResult(url, htmlContent, opts)
}

// buildStaticResult turns raw HTML into the standard scrape result. It is
// shared with the solver path so a challenge addon that returns post-challenge
// HTML produces exactly the same shape as a normal static scrape.
func buildStaticResult(url, html string, opts domain.ScrapeOptions) (*domain.ScrapeResult, error) {
	clean := cleanContent(html,
		opts.BlockAds == nil || *opts.BlockAds,
		opts.RemoveBase64Images == nil || *opts.RemoveBase64Images,
	)

	// Extract the main content after cleaning so nav/ads/footers don't pollute
	// the LLM-ready output. Falls back to full HTML on failure.
	rc, _ := ExtractMainContent(clean, url)

	markdown, err := md.ConvertString(rc.ContentHTML)
	if err != nil {
		return nil, fmt.Errorf("markdown conversion failed: %w", err)
	}

	metadata := map[string]string{
		"scraped_at": time.Now().Format(time.RFC3339),
		"engine":     "colly",
	}
	applyReadabilityMetadata(metadata, rc)

	links := []domain.LinkData{}
	if opts.IncludeLinks == nil || *opts.IncludeLinks {
		links = ExtractLinks(rc.ContentHTML, url)
	}

	return &domain.ScrapeResult{
		URL:      url,
		Markdown: markdown,
		HTML:     html,
		Metadata: metadata,
		Links:    links,
	}, nil
}

// requestURL reads the URL Colly attached to a response without assuming the
// request survived (it is absent on some transport-level failures).
func requestURL(r *colly.Response) string {
	if r.Request != nil && r.Request.URL != nil {
		return r.Request.URL.String()
	}
	return ""
}

// challengeBody keeps the challenge page small enough to hand to a solver
// addon without letting a hostile origin balloon memory.
func challengeBody(body []byte) string {
	if len(body) > challengeMaxBody {
		body = body[:challengeMaxBody]
	}
	return string(body)
}
