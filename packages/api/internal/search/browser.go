package search

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Engine names accepted by the browser backend.
const (
	BrowserDDG    = "ddg"
	BrowserGoogle = "google"
	BrowserBing   = "bing"
)

// BrowserEngines lists the engines this backend can read, in the order a caller
// should try them: DuckDuckGo first (no account to burn), Google second (best
// results, the one that can be challenged), Bing last.
var BrowserEngines = []string{BrowserDDG, BrowserGoogle, BrowserBing}

// FetchAction is one page interaction for a fetcher that can drive the browser.
// It mirrors the bridge's action vocabulary, redefined here so this package keeps
// its rule of importing nothing from the project.
// Only the fields domain.Action can carry travel to the browser. A consent click
// needs a selector and a wait, and pretending the rest is available would be a
// silent drop.
type FetchAction struct {
	Type     string
	Selector string
	Script   string
	Ms       int
}

// ActionFetcher is a BrowserFetcher that can also drive the page before reading
// it. Exactly one thing needs this: clearing a consent wall by clicking the
// control a person would click. A read-only fetcher still works — it just cannot
// get past the wall, and says so.
type ActionFetcher interface {
	BrowserFetcher
	FetchHTMLWithActions(ctx context.Context, url string, actions []FetchAction) (string, error)
}

// ErrBrowserChallenge means the engine served its own interstitial ("unusual
// traffic", "are you a robot"). It is not a parse failure and not a network
// failure: a human has to clear the tab. The chain treats it as one more failed
// rung rather than retrying, because retrying is what turns a warning into a
// suspension.
var ErrBrowserChallenge = errors.New("search: the engine served a challenge page to the browser")

// BrowserService reads a search engine's results page through the user's own
// signed-in browser.
//
// It exists because of measurement, not preference: the engine telemetry for
// 2026-11-1x shows DuckDuckGo returning `blocked` (anomaly-modal) 40 times
// against 30 successes, and Marginalia 77 blocked against 10 ok, from
// Tomoshibi's own egress. Those engines are not broken; they are locked out of
// their own house. The same request leaving from the browser the user already
// uses is the access the engine expects.
//
// It implements Service, and is meant to sit in the chain *before* the paid
// API: free first, money last.
type BrowserService struct {
	fetcher BrowserFetcher
	engine  string
}

// NewBrowserService wires the engine to a browser fetcher. An unknown engine
// name falls back to DuckDuckGo rather than failing the whole search.
func NewBrowserService(fetcher BrowserFetcher, engine string) *BrowserService {
	e := strings.ToLower(strings.TrimSpace(engine))
	switch e {
	case BrowserGoogle, BrowserBing, BrowserDDG:
	default:
		e = BrowserDDG
	}
	return &BrowserService{fetcher: fetcher, engine: e}
}

// Engine reports which engine this instance reads.
func (s *BrowserService) Engine() string { return s.engine }

// engineFor resolves the engine for one request: the caller's pick wins, then
// this instance's default.
func (s *BrowserService) engineFor(opts SearchOptions) string {
	pick := strings.ToLower(strings.TrimSpace(opts.BrowserEngine))
	switch pick {
	case BrowserGoogle, BrowserBing, BrowserDDG:
		return pick
	}
	return s.engine
}

// serpURL builds the results URL for an engine. `udm=14` on Google is the Web
// filter: it drops the AI overview and the sponsored blocks that make the SERP a
// moving target, which is both a quality win and a stability win.
func serpURL(engine, query string, limit, offset int) string {
	q := url.Values{}
	q.Set("q", query)
	switch engine {
	case BrowserGoogle:
		u := "https://www.google.com/search?" + q.Encode()
		u += "&udm=14"
		if limit > 0 {
			u += "&num=" + strconv.Itoa(limit)
		}
		if offset > 0 {
			u += "&start=" + strconv.Itoa(offset)
		}
		return u
	case BrowserBing:
		u := "https://www.bing.com/search?" + q.Encode()
		// Ask for the RSS view: it is XML, it is not client-rendered, and Bing
		// has shipped it for a decade. Scraping the HTML SERP is a moving target
		// (Bing renders results with script) while this is a list of items.
		if limit > 0 {
			u += "&count=" + strconv.Itoa(limit)
		}
		if offset > 0 {
			u += "&first=" + strconv.Itoa(offset+1)
		}
		return u
	default:
		u := "https://html.duckduckgo.com/html?" + q.Encode()
		if offset > 0 {
			u += "&s=" + strconv.Itoa(offset)
		}
		return u
	}
}

// Search fetches one results page and parses it.
func (s *BrowserService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
	if err := ValidateCategory(opts.Category); err != nil {
		return nil, 0, err
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, 0, fmt.Errorf("search: empty query")
	}
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if s.fetcher == nil {
		return nil, 0, fmt.Errorf("search: the browser backend needs a browser fetcher (pairing the bridge extension enables it)")
	}

	pageURL := serpURL(s.engineFor(opts), opts.Query, opts.Limit, opts.Offset)
	engine := s.engineFor(opts)
	html, err := s.fetcher.FetchHTML(ctx, pageURL)
	if err != nil {
		return nil, 0, fmt.Errorf("search: %s through the browser: %w", engine, err)
	}

	raw := parseForEngine(engine, html)

	// A consent wall is the one thing a plain read cannot get past, and Google
	// serves one to any profile that has not answered it yet. The bridge can
	// click the button a person would click, which is also the honest way to
	// answer it: the same choice, made by the human who owns the browser, once.
	// After that the cookie is in the profile and later searches are clean.
	if len(raw) == 0 && isConsentWall(html) {
		if af, ok := s.fetcher.(ActionFetcher); ok {
			if cleared, cerr := af.FetchHTMLWithActions(ctx, pageURL, consentActions(engine)); cerr == nil && cleared != "" {
				html = cleared
				raw = parseForEngine(engine, html)
			}
		}
	}

	results := make([]Result, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		link := strings.TrimSpace(r.url)
		link = unwrapRedirect(link)
		if link == "" || seen[link] {
			continue
		}
		if skipURL(link) {
			continue
		}
		seen[link] = true
		results = append(results, Result{
			Title:       r.title,
			URL:         link,
			Description: r.snippet,
			Domain:      domainOf(link),
			Engine:      "browser:" + engine,
		})
		if len(results) >= opts.Limit {
			break
		}
	}

	// Challenge detection comes AFTER parsing, and only when parsing found
	// nothing. Every engine ships the words "anomaly-modal" and "unusual traffic"
	// in an inline script on perfectly good result pages, so checking first
	// refuses pages that plainly have their results on them. A page with results
	// is not a challenge, whatever it says about itself.
	if len(results) == 0 {
		if looksChallenged(html) {
			return nil, 0, fmt.Errorf("%w (%s)", ErrBrowserChallenge, engine)
		}
		// Say what actually came back. "No parseable results" on its own is a
		// dead end; the title and the first words of the page are usually enough
		// to see that it was a consent wall, a redirect, or a login prompt.
		return nil, 0, fmt.Errorf(
			"search: %s returned a page with no parseable results: %s",
			engine, describePage(html),
		)
	}
	return results, len(results), nil
}

// parseForEngine is the one place the engine-to-parser mapping lives, so the
// consent retry cannot end up parsing with a different parser than the first
// attempt.
func parseForEngine(engine, html string) []rawResult {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	switch engine {
	case BrowserGoogle:
		return parseGoogle(doc)
	case BrowserBing:
		// The HTML list. Bing's `format=rss` view was tried and dropped: a browser
		// renders XML as a document, and `<link>` is a void element in HTML, so the
		// item's URL is destroyed before it reaches a parser.
		return parseBing(doc)
	default:
		return parseDuckDuckGo(doc)
	}
}

// describePage is the self-diagnosing half of the "nothing parsed" error: the
// title plus the first visible words. It is bounded so an error never carries a
// page.
func describePage(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "unparseable HTML"
	}
	title := cleanText(doc.Find("title").First().Text())
	doc.Find("script, style, noscript").Remove()
	text := cleanText(doc.Find("body").Text())
	if len(text) > 180 {
		text = text[:180] + "…"
	}
	if title == "" && text == "" {
		return "empty page"
	}
	return fmt.Sprintf("title=%q text=%q", title, text)
}

// isConsentWall reports whether the page is a consent/privacy interstitial rather
// than results. It matches the shape of the wall (the host, or the words every
// consent page uses), not a specific engine's markup.
func isConsentWall(html string) bool {
	h := strings.ToLower(html)
	for _, marker := range []string{
		"consent.google.com",
		"/consent?",
		"before you continue",
		"before you continue to google",
		"we use cookies and data to",
		"your privacy choices",
		"accept all",
		"reject all",
	} {
		if strings.Contains(h, marker) {
			return true
		}
	}
	return false
}

// consentActions is the click a person makes once. Only explicit, labelled
// controls are named — never "the other button" — because the wrong click here
// is a privacy decision made on someone's behalf. Each click is advisory: a
// selector that is not on the page fails, the rest still run.
func consentActions(engine string) []FetchAction {
	if engine != BrowserGoogle {
		return nil
	}
	return []FetchAction{
		// Give the wall a moment to finish rendering before poking it.
		{Type: "wait_selector", Selector: "#L2AGLb, button[aria-label*='Accept all'], form[action*='consent']", Ms: 6000},
		// The classic "Accept all" control, then the modern labels. All of them
		// name accepting explicitly.
		{Type: "click", Selector: "#L2AGLb"},
		{Type: "click", Selector: "button[aria-label='Accept all']"},
		{Type: "click", Selector: "button[aria-label*='Accept all']"},
		{Type: "click", Selector: "button[jsname='b3VHJd']"},
		// Wait for whichever page follows, then let it settle.
		{Type: "wait_ms", Ms: 1500},
	}
}

type rawResult struct {
	title   string
	url     string
	snippet string
}

// parseDuckDuckGo reads the html.duckduckgo.com layout: one `.result` block per
// hit with `.result__a` and `.result__snippet`. The links are redirect wrappers
// (`/l/?uddg=...`), unwrapped later.
func parseDuckDuckGo(doc *goquery.Document) []rawResult {
	var out []rawResult
	collect := func(container string, titleSel string) {
		doc.Find(container).Each(func(_ int, sel *goquery.Selection) {
			link := sel.Find(titleSel).First()
			if link.Length() == 0 {
				link = sel.Find("a[href]").First()
			}
			href, _ := link.Attr("href")
			if href == "" {
				return
			}
			out = append(out, rawResult{
				title:   cleanText(link.Text()),
				url:     href,
				snippet: cleanText(sel.Find(".result__snippet, [data-result=\"snippet\"]").First().Text()),
			})
		})
	}
	// The classic HTML endpoint, then the newer markup. Both are structural
	// ("one container per hit"), which is what survives a class-name rename.
	collect(".result", "a.result__a")
	if len(out) == 0 {
		collect("article[data-testid='result']", "[data-testid='result-title-a']")
	}
	return out
}

// collectResultLinks is the parser both engines use: walk the containers from
// most specific to least, and take the first pass that produced anything. A link
// counts as a result when it wraps a heading (the shape every engine has used for
// a decade) or when its own text is a title-length phrase (the newer,
// div-and-link layouts).
//
// Both engines' own URLs are filtered downstream, so the `body` fallback cannot
// turn a navigation bar into results: "Images", "Videos" and "AI Mode" all point
// at the engine itself.
func collectResultLinks(doc *goquery.Document, containers []string) []rawResult {
	var out []rawResult
	seen := map[string]bool{}

	collect := func(root string) {
		doc.Find(root).Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
			href, _ := sel.Attr("href")
			if href == "" {
				return
			}
			// Two heading shapes: the anchor wraps it (<a><h3>..</h3></a>) or the
			// heading wraps the anchor (<h2><a>..</a></h2>). Both are "a result".
			title := cleanText(sel.Find("h1, h2, h3, h4").First().Text())
			if title == "" && sel.Parent().Is("h1, h2, h3, h4") {
				title = cleanText(sel.Parent().Text())
			}
			if title == "" {
				txt := cleanText(sel.Text())
				if len(txt) < 12 || len(txt) > 140 {
					return
				}
				title = txt
			}
			if !strings.HasPrefix(href, "http") && !strings.HasPrefix(href, "/url?") {
				return
			}
			if seen[href] {
				return
			}
			seen[href] = true
			out = append(out, rawResult{
				title:   title,
				url:     href,
				snippet: nearestSnippet(sel, title),
			})
		})
	}

	for _, root := range containers {
		collect(root)
		if len(out) > 0 {
			return out
		}
	}
	return out
}

// parseGoogle is structural on purpose: Google's class names are obfuscated and
// rotate, and the container id depends on which filter is in use (`#rso` for the
// Web filter, `#search` and `#main` for others).
func parseGoogle(doc *goquery.Document) []rawResult {
	return collectResultLinks(doc, []string{"#rso", "#search", "#main", "body"})
}

// parseBing reads the same shape. `li.b_algo` is tried first because it is exact
// when present; the containers cover the layout that replaces it.
func parseBing(doc *goquery.Document) []rawResult {
	var out []rawResult
	doc.Find("li.b_algo").Each(func(_ int, sel *goquery.Selection) {
		link := sel.Find("h2 a[href]").First()
		if link.Length() == 0 {
			link = sel.Find("a[href]").First()
		}
		href, _ := link.Attr("href")
		if href == "" {
			return
		}
		out = append(out, rawResult{
			title:   cleanText(link.Text()),
			url:     href,
			snippet: cleanText(sel.Find(".b_caption p, .b_lineclamp2, p").First().Text()),
		})
	})
	if len(out) > 0 {
		return out
	}
	return collectResultLinks(doc, []string{"#b_results", "#results", "main", "body"})
}

// nearestSnippet walks up from a result link until an ancestor's text is longer
// than the title but still short enough to be a snippet. Google puts the snippet
// in an unlabelled div, so structure is the only handle available.
func nearestSnippet(node *goquery.Selection, title string) string {
	snippet := ""
	node.ParentsFiltered("div").EachWithBreak(func(_ int, p *goquery.Selection) bool {
		t := cleanText(p.Text())
		if len(t) > len(title)+20 && len(t) < 400 {
			snippet = strings.TrimSpace(strings.TrimPrefix(t, title))
			return false
		}
		return true
	})
	return snippet
}

// looksChallenged spots the interstitials the engines serve instead of results.
// They arrive as HTTP 200, so status alone never tells you.
func looksChallenged(html string) bool {
	h := strings.ToLower(html)
	for _, marker := range []string{
		"unusual traffic",
		"are you a robot",
		"automated queries",
		"anomaly-modal",
		"enable javascript and cookies to continue",
		"checking your browser before accessing",
		"verifying you are human",
	} {
		if strings.Contains(h, marker) {
			return true
		}
	}
	return false
}

// unwrapRedirect turns an engine's own redirect wrapper into the real target.
// Fetching the wrapper instead of the page is both slower and exactly the
// pattern "an automated agent clicked a result", which is the wrong thing to be.
func unwrapRedirect(href string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	if u, err := url.Parse(href); err == nil {
		// url.Parse already decodes the query, so `uddg` is the target as-is.
		// Decoding it a second time would corrupt a target that legitimately
		// contains an escape (a path like /a%2Fb).
		if target := u.Query().Get("uddg"); target != "" {
			return target
		}
		if strings.HasPrefix(u.Path, "/url") {
			if target := u.Query().Get("q"); strings.HasPrefix(target, "http") {
				return target
			}
			if target := u.Query().Get("u"); strings.HasPrefix(target, "http") {
				return target
			}
		}
		// Bing wraps every result in its own click tracker. The visible URL and
		// the href disagree, and leaving the wrapper in place means either
		// fetching a tracker redirect (which is exactly the "an agent clicked a
		// result" pattern) or dropping the result as an engine self-link.
		if strings.HasPrefix(u.Path, "/ck/a") {
			if target := u.Query().Get("u"); target != "" {
				if decoded := decodeBingTarget(target); decoded != "" {
					return decoded
				}
			}
		}
	}
	return href
}

// skipURL drops links that are never a result: the engine's own pages, its
// consent walls, and non-http schemes.
func skipURL(href string) bool {
	if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
		return true
	}
	u, err := url.Parse(href)
	if err != nil {
		return true
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return true
	}
	for _, blocked := range []string{
		"google.com", "www.google.com", "googleusercontent.com", "gstatic.com",
		"duckduckgo.com", "html.duckduckgo.com",
		"bing.com", "www.bing.com", "microsofttranslator.com",
	} {
		if host == blocked {
			return true
		}
	}
	// Google's own consent and account surfaces.
	if strings.Contains(u.Path, "/policies") || strings.Contains(u.Path, "/accounts") {
		return true
	}
	return false
}

// domainOf is the bare hostname of a result, for the Domain field.
func domainOf(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// cleanText collapses whitespace, which is the difference between a usable
// title and a block of newlines.
func cleanText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// decodeBingTarget unpacks Bing's `u` parameter: an "a1" marker followed by the
// real URL in unpadded base64url. Anything that does not decode to an http(s) URL
// returns "", so a format change degrades to "leave the link alone" rather than
// to a wrong URL.
func decodeBingTarget(raw string) string {
	body := strings.TrimPrefix(raw, "a1")
	if body == raw && !strings.HasPrefix(raw, "http") {
		return ""
	}
	if strings.HasPrefix(raw, "http") {
		return raw
	}
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
	} {
		if decoded, err := enc.DecodeString(body); err == nil {
			if s := string(decoded); strings.HasPrefix(s, "http") {
				return s
			}
		}
	}
	return ""
}
