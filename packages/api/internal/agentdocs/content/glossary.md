# Glossary

The vocabulary this project uses in code, docs, and issues. Terms are defined
by what they mean to a user of Tomoshibi, not by how they are implemented.

## Scrape

Fetching one URL and returning its content, usually as markdown, with optional
enrichment (links, images, schema extraction, summary).

## Scrape mode

How a scrape obtains the page.

- **Static** — fetch the HTML over HTTP without running JavaScript.
- **Dynamic** — render the page in a real browser and read the resulting DOM.
- **Smart** — try static first, then switch to dynamic when the page needs it.

## Challenge

An anti-bot interstitial (Cloudflare, DataDome, PerimeterX, Imperva, Akamai,
AWS WAF, Sucuri, DDoS-Guard) served instead of the requested page. Challenges
are often returned with HTTP 200, so they cannot be detected from the status
code alone.

## Bridge

The loopback link between the API and a paired browser extension. When a recipe
allows it, a scrape is handed to the **borrow** lane instead: the page is loaded
and read inside the user's own signed-in browser, which is the only way through
some sites. The bridge never exports cookies, never types credentials, and never
reads a tab it was not asked about; it is loopback-only (a web page in the same
browser is refused by origin), and the extension will not release its token to a
process that cannot prove it holds the key from pairing.

## Lane

How a recipe says a site should be fetched.

- **Plain** — the normal engine pipeline (HTTP, then a headless browser). No
  session, no account, nothing borrowed.
- **Borrow** — through the bridge, in the user's own browser.
- **Official** — the site's own documented API, when it has one.
- **Import** — the site's data copied into a persistent Tomoshibi browser
  profile once, so later reads need no session (planned; today the lane fails
  loudly rather than pretending).

A lane that is declared but not implemented fails loudly rather than falling
back to a different one.

## Recipe

One site's access definition: which hosts it covers, the lane, the IO layer, the
page actions to run before reading, and the fixtures that prove it still works.
Recipes are installed as **packs** (a manifest hashing every file), verified
offline against saved pages, and matched to a request by host — most specific
wins, and an explicit `recipe` parameter beats the automatic match.

## IO layer

How the bridge drives the page.

- **Scripting** — ordinary DOM events through the extension API. No debugger, no
  infobar, works on Chrome and Firefox.
- **Turbo (`io: turbo`)** — Chrome's debugger protocol: trusted input events and
  full-page screenshots. Chrome only, and the tab shows a debugging notice.

## Provenance

The `provenance` block on a scrape result: which transport served it, the lane,
the recipe, the IO layer, how many actions ran, how long it took. Present only
when a recipe routed the fetch.

## Untrusted

A page fetched through the bridge is marked `untrusted: true`. It is what the
site served to a signed-in human, so it may contain text written to be mistaken
for instructions. Treat it as data.

## Blocked response

The outcome when Tomoshibi detects a challenge and cannot recover the real
page. It is an error, never content: the body is a block page, not the page the
caller asked for.

## Harvest / extracted data

Embedded payloads found in the rendered document — `__NEXT_DATA__`, Nuxt
state, Next flight data, JSON-LD — returned under `extracted_data`. Harvest
sees only what the page serialized into its HTML.

## Network capture

Opt-in recording of the XHR/fetch responses a page made while it was rendered,
returned under `network`. It sees runtime API traffic that harvest cannot.

## Captured response

One recorded XHR/fetch exchange: URL, method, status, content type, resource
type, body, size, and flags for truncation, cache, and redirects.

## Crawl

Following links from a seed URL and scraping each page, within a depth and page
limit.

## Politeness floor

The minimum delay between requests to the same host during a crawl. It is a
floor, not a fixed delay: the crawler may slow down further when a site pushes
back.

## Engine

The component that performs a fetch. The static engine speaks HTTP; the dynamic
engine drives a browser.
