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
