# Full-page screenshots — design

**Date:** 2026-09-16
**Status:** Implemented (backend, MCP, web playground)
**Supersedes:** ad-hoc `full_page` flag behavior from the v2 image/screenshot work

## Problem

`screenshot: true` on `POST /v1/scrape` captured whatever fit the viewport, and the capture fired as soon as `body` was visible. On JS-rendered pages that means the screenshot usually documents a loading state, and there was no single-call way to get the whole page.

## Decision: one long capture, not scroll-and-stitch

Two candidate implementations were researched and tested against the pinned toolchain (Chrome 153, chromedp v0.14.2, headless, software rendering):

1. **One long capture** — CDP `Page.captureScreenshot` with `captureBeyondViewport: true`, clipped to the document's CSS content size.
2. **Scroll and stitch** — capture viewport-by-viewport and compose an image in Go.

Option 1 was chosen. It is one CDP call, produces no seams, renders `position: fixed` elements once at the top (verified by pixel probe), and — on current Chrome — has no practical height ceiling. Option 2 would add an image-composition subsystem (new dependency, overlap math, per-frame settle) whose only advantage is triggering lazy loaders naturally, which the pre-scroll pass in option 1 already covers.

### Empirical findings (local probe, then end-to-end smoke test)

| Scenario                                 | Result                                                         |
| ---------------------------------------- | -------------------------------------------------------------- |
| 4 000 px page, `captureBeyondViewport`   | 1905×4000 in one call                                          |
| 26 000 px page, `captureBeyondViewport`  | 1905×26000, 2.8 s, 196 KB PNG — no 16384 px clip on Chrome 153 |
| `position: fixed` header                 | rendered once at top (correct)                                 |
| Lazy block below the fold, no scroll     | **never loads** — capture shows the placeholder                |
| Same page after a stepped pre-scroll     | lazy content loaded and present in the capture                 |
| Clip chunk at y=16000 (chunked fallback) | correct content                                                |

Historical context: Chromium hard-capped captures at 16384 px for years (crbug.com/770769, Puppeteer #3202/#5341). That is why a configurable cap exists even though current Chrome does not need it — production runs Debian `chromium-browser`, not Chrome 153.

## Behavior spec

1. **Full page is the default.** `screenshot: true` captures the whole scrollable page. `screenshot_opts.full_page: false` opts back to a viewport capture. The wire type is `*bool` so "unset" is distinguishable from an explicit `false`.
2. **A capture never races the page.** Before capturing, the server waits for `document.readyState === "complete"` _and_ the network to be quiet (no in-flight requests for 500 ms), bounded by a 15 s budget. Pages that never go quiet (websockets, long-polling) log a warning and proceed at the deadline.
3. **Lazy content is loaded first.** Full-page captures walk the page down ~one viewport at a time (150 ms settle per step, 40 steps max), return to the top, then wait once more (10 s budget) for the traffic that scroll triggered.
4. **The capture viewport is set before navigation**, so responsive breakpoints and lazy-loading decisions match the final image.
5. **Height is capped and honest.** The capture height is `min(cssContentSize.height, APP_SCREENSHOT_MAX_HEIGHT)` and is never smaller than the requested viewport. When clipped, `screenshot.truncated` is `true`.
6. **Response metadata reports what happened:** `format`, `width`, `height`, `full_page`, `truncated`, `size_bytes`, `captured_at` — populated by decoding the captured image header.
7. **Screenshot results are never cached** (large blobs, time-sensitive), but they **do** run the full post-processing pipeline (extract_schema, summary, PII redaction, images) — earlier the smart-mode shortcut skipped enrichment entirely.
8. Screenshot failure remains non-fatal: the scrape returns HTML/markdown with a logged warning.

## Configuration

| Variable                    | Default | Range             | Meaning                                    |
| --------------------------- | ------- | ----------------- | ------------------------------------------ |
| `APP_SCREENSHOT_MAX_HEIGHT` | `16384` | 1–32768 (clamped) | Full-page capture height cap in CSS pixels |

## Implementation map

- `internal/domain/media.go` — `ScreenshotOptions.FullPage *bool`, `ScreenshotData.Truncated`.
- `internal/api/handlers/scrape.go` — wire `full_page` pointer passthrough.
- `internal/config/config.go` — `app.screenshot_max_height` (+ `.env.example`, `plan/env.example`).
- `internal/scraper/chromedp.go` — `prepareScreenshot` (ready + idle + pre-scroll), `capturePage` (metrics → clamp → clip), `requestTracker` (in-flight counter via CDP network events), `waitForPageReady`, `autoScroll`, `screenshotDimensions`; constructors gain `NewChromedpScraperWithConfig`.
- `internal/scraper/service.go` — smart mode routes screenshots to dynamic without skipping enrichment; cache write skipped when `Screenshot` is set.
- `packages/mcp` — types (`truncated`), tool schema descriptions, output lines with dimensions/clipping.
- `packages/web` — `full_page` in the scrape schema, nested `screenshot_opts` payload, Full page toggle, hidden input, `Truncated` badge in the viewer.

## Verification

- Unit: params defaults/opt-out, cap clamps, tracker bookkeeping (`internal/scraper/chromedp_test.go`); pipeline test asserts enrichment still runs for screenshot requests (`service_pipeline_test.go`).
- End-to-end: real server + local Chrome — example.com (1080 px page), gnu.org, and en.wikipedia.org (1920×8722 in ~14 s, ~2.8 MB JPEG).
- Svelte components validated with `@sveltejs/mcp svelte-autofixer`; web `svelte-check` and MCP `tsc --noEmit` clean.

## Follow-ups

- MCP could return the screenshot as an MCP `image` content block, not just a text line.
- Crawl/async payloads (`ScrapePayload`) still carry only `Screenshot bool`; exposing `screenshot_opts` there would give crawl users the same control.
- If operators need pages beyond 32768 px, chunked `captureBeyondViewport` clips + stitch is the tested-worthy path (clip chunks below the fold already render correctly).
