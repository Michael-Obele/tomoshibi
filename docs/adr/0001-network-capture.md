# 0001 — Network capture returns flat XHR responses, opt-in

- **Status:** Accepted
- **Date:** 2026-11-04
- **Deciders:** Tomoshibi maintainers

## Context

The dynamic engine renders a page and returns markdown. For single-page apps,
the data a caller actually wants often never appears in the DOM: it arrives in
an XHR/fetch response that the page's JavaScript renders. The existing
`extracted_data` harvest covers embedded payloads (`__NEXT_DATA__`, Nuxt,
Next flight, JSON-LD), but it cannot see responses that were fetched at
runtime and never serialized into the document.

A naive implementation is dangerous in three ways. Chrome retains response
bodies in the renderer (its desktop default is 200 MB total / 20 MB per
resource, per tab). Calling `Network.getResponseBody` inside a CDP listener
deadlocks the target loop, and calling it at `ResponseReceived` returns partial
bodies. Captured bodies routinely contain bearer tokens, API keys, session
IDs, and PII.

## Decision

Add an opt-in `capture_network` scrape option. When enabled:

1. **Mechanism:** passive CDP `Network` events only. One listener on the
   per-tab context, registered before navigation. Bodies are fetched on
   `LoadingFinished` by exactly one bounded goroutine. `Fetch.enable` is
   rejected: pausing requests can hang page actions and screenshots.
2. **Scope:** `XHR` and `Fetch` resource types whose MIME is text-like
   (`application/json`, `+json`, NDJSON, XML, `text/plain`, CSV,
   `x-www-form-urlencoded`). Documents, images, scripts, fonts, WebSockets and
   EventSource are not captured.
3. **Shape:** a flat `network.responses` array:
   `{url, method, status, content_type, resource_type, body, size_bytes,
   truncated, from_cache, redirected, count, body_error}`. Bodies stay raw
   strings; nothing is parsed on the server. HAR is not the default.
4. **Privacy:** request headers, cookies and `Set-Cookie` are never recorded.
   Query parameters whose names look like credentials are redacted. Bodies are
   untrusted input and are returned in their own field, never merged into
   `markdown`.
5. **Limits:** 100 responses, 512 KiB per body, 8 MiB total, deduplicated by
   `SHA-256(method + URL + status + body)` with a repeat count. Chrome's own
   buffers are pinned to 16 MiB total / 1 MiB per resource.
6. **Lifecycle:** capture is off by default, requires dynamic mode (smart mode
   forces dynamic; static mode is an error), is never cached in Redis, and is
   drained after page actions and after the screenshot so late and lazy-loaded
   requests are included.

## Consequences

**Positive**

- Callers get the API data behind a rendered page without re-discovering and
  re-requesting endpoints themselves.
- Limits are enforced before the body enters the Go heap, so a request-heavy
  page cannot push the process past its 512 MB target.
- The response shape is small and agent-friendly; no replay metadata or
  cookie jars.

**Negative**

- Capture costs memory and a listener per tab. That is why it is opt-in.
- Redirect bodies are lost: CDP reuses the `RequestID` and only the final hop
  survives. `redirected` records that a redirect happened, not the chain.
- Cached, streamed, aborted, and pre-`Network.enable` requests may have no
  body; the entry is kept with `body_error` rather than dropped silently.
- HAR consumers must wait for a future export; flat records are not a
  drop-in replacement for replay tooling.

## Alternatives considered

- **HAR as the default shape.** Rejected: nested, carries replay metadata and
  cookies, and has no inherent body bound. It remains the right format for a
  debugging export, not for an API response consumed by an agent.
- **`Fetch.enable` interception.** Rejected: it pauses every request until the
  collector responds, which can hang page actions and screenshots. Passive
  events give the same data without changing page behavior.
- **Re-requesting endpoints discovered in the DOM.** Rejected: it duplicates
  requests, misses requests that need page state or auth headers, and turns a
  scrape into an unbounded crawler.
- **Scrapling's larger ceilings (1,000 / 1 MiB / 20 MiB).** Rejected for the
  default: they assume a larger host. The smaller caps are the safe default
  for the deployment this project targets.

## References

- Local research notes: `plan/network-capture/research.md`
- CDP Network domain: <https://chromedevtools.github.io/devtools-protocol/tot/Network/>
- Chromium network agent (buffer defaults, body errors):
  <https://github.com/chromium/chromium/blob/main/third_party/blink/renderer/core/inspector/inspector_network_agent.cc>
- HAR 1.2 spec (privacy warning): <http://www.softwareishard.com/blog/har-12-spec/>
- OWASP LLM01, prompt injection: <https://genai.owasp.org/llmrisk/llm01-prompt-injection/>
