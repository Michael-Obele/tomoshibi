# Tomoshibi Bridge

A small browser extension that lets your local [Tomoshibi](https://github.com/Michael-Obele/tomoshibi)
read and drive pages in the browser you are already signed into.

Tomoshibi cannot reach sites that know you: no cookies, a datacenter IP, a Go
TLS stack pretending to be Chrome. Modern bot protection scores whether all
those layers *agree*. The honest fix is not a sneakier bot; it is to be your
browser. That is all this extension does.

## What It Does

The extension opens one loopback WebSocket to Tomoshibi on `127.0.0.1` and
answers `fetch` messages by driving a real tab and reading it back:

- **Scripting** (default) — click, type, keys, scroll, navigate through the
  extension scripting API. No debugger, no infobar, no CDP tells; works on
  Firefox too.
- **turbo** (per-recipe `io: turbo`) — attaches `chrome.debugger` for trusted
  CDP input and full-page screenshots. Chrome only; shows the debugging infobar
  while a fetch runs.

A challenge page (Cloudflare, Turnstile, a login wall) stops the run as
*awaiting-human*: the tab stays open, you clear it, and the next fetch
proceeds. The bridge never solves a challenge and never types credentials.

## What it will not do

- No cookie access: `cookies`, `webRequest` and `nativeMessaging` are not in
  either manifest, and `bun test/e2e-protocol.ts` asserts they stay out.
- No credential typing, ever.
- No proxy rotation, no account pools, no solver.
- No reading tabs it was not asked about; one pinned tab per origin, opened on
  demand.
- No answering a server it has not verified: the extension releases its token
  only after the process on the port proves it holds the key from pairing.

## Install

1. Start Tomoshibi (the bridge is built in):
   `go run ./cmd/api` from `packages/api`, or the Docker image.
2. Build this directory: `bun install && bun run build`. It compiles to
   `.output/chrome-mv3` (Firefox: `bun run build:firefox` →
   `.output/firefox-mv3`).
3. Load the build as an unpacked extension: `chrome://extensions` → Developer
   mode → **Load unpacked** → pick `.output/chrome-mv3` (not this directory — it
   holds the sources, not the extension).
4. Run `tomoshibi bridge code` (or read the startup log) and type the 6-digit
   code into the extension popup. One pairing per browser; the token is stored
   locally and revoked with `tomoshibi bridge unpair` (or by deleting
   `~/.config/tomoshi/bridge.json`, which the running server notices).

You are now ready. **A recipe is not required.**

## Recipe-free use (the normal path)

Ask for the browser on the request itself:

```bash
curl -s -X POST http://127.0.0.1:7431/v1/scrape \
  -H 'content-type: application/json' \
  -d '{"url":"https://example.com/dashboard","transport":"bridge"}'
```

- `transport: "bridge"` drives the browser. `"auto"` tries the anonymous lanes
  first and escalates into the browser only when the answer is a block or a shell.
- An origin the server does not know yet answers **428 `consent-required`** with
  the origin. That is the gate, not a bug:

  ```bash
  tomoshibi bridge allow example.com      # or *.example.com
  tomoshibi bridge allowed                # what is allowed now
  tomoshibi bridge deny example.com
  ```

  A fresh install already allows the three search hosts (DuckDuckGo, Google,
  Bing) and nothing else — deliberately not `*.google.com`, which would include
  mail.
- `tomoshibi bridge status` shows the pairing and the allowlist.
- A page budget (600 pages/hour, `BRIDGE_PAGES_PER_HOUR`) caps a runaway loop.

Recipes are the specialization, not the price of admission: use one when a site
needs its own actions, its own extraction schema, its own pace, or `io: turbo`.

## Search without a search API

The same browser rung turns the engines that block a datacenter address back on.
The engine telemetry in `data/telemetry` is the reason this exists: DuckDuckGo
returns `blocked` more often than `ok` from this machine's own egress.

```bash
# switch engines per request
curl -s -X POST http://127.0.0.1:7431/v1/search \
  -H 'content-type: application/json' \
  -d '{"query":"svelte 5 runes","engine":"browser:ddg"}'

# or ask for pages, not just links
curl -s -X POST http://127.0.0.1:7431/v1/research \
  -H 'content-type: application/json' \
  -d '{"query":"svelte 5 runes","engine":"browser:google","pages":5,"summary":true}'
```

`browser:ddg`, `browser:google` and `browser:bing` are accepted; omit `engine`
for the configured chain, which puts the free engines first and the paid API
last. `research` returns the SERP and the read pages together, with
`paid_api_used` telling you what it actually cost.

The popup also holds the API port (default `7431`), the pairing code box, and a
pause switch. The toolbar badge says which state it is in: green `on` means
paired and connected, amber `pair` means connected but unpaired, grey-red `off`
means no connection, red `err` means the last attempt failed (the popup shows
why).

The manifest asks for `host_permissions: ["<all_urls>"]`, because a recipe may
name any host. Nothing is read until a fetch names a URL.

Firefox: build it with `bun run build:firefox` and load
`.output/firefox-mv3` through `about:debugging` → This Firefox → Load Temporary
Add-on. The Firefox build has no `debugger` permission, so `io: turbo` fails with a
readable message and the scripting layer keeps working. The Firefox
manifest drops the `debugger` permission, so `io: turbo` recipes fail with a
clear message and the scripting layer keeps working.

## Security notes

- The socket is loopback-only, and Tomoshibi refuses any `http(s)` origin: a
  web page in your own browser is a loopback peer too, so only extension
  origins (and clients that send no Origin at all) are accepted.
- Pairing is mutual. The code rotates every 10 minutes, works once, and ten
  wrong guesses burn it; the extension verifies the server's proof with the
  code you typed before it stores anything.
- Every fetch is written to Tomoshibi's `access.log.jsonl`: recipe, lane,
  origin, result.
- Fetches refuse to drive a page that has left the origin they asked for, and a
  scripting-layer screenshot is skipped if another tab becomes active first.
- `debugger` permission exists for the turbo layer only. If you never use
  `io: turbo` recipes, delete it from `wxt.config.ts` and drop `runTurbo` from
  `entrypoints/background/index.ts`.

## Development

WXT builds it. `bun run dev` starts Chrome with the extension loaded and HMR;
`bun run build` writes `.output/chrome-mv3`, which is what you load and what the
E2E suites load. `bun run check` is the typecheck plus the unit tests. There is no
icon yet, so the toolbar shows the default puzzle piece.

`AGENTS.md` is the contract for anyone changing this repo: the commands, the
rules that tests enforce, and what a change here forces on the Tomoshibi side.
`docs/product.md` is the plan for turning this spike into the product.

```bash
bun test/e2e-protocol.ts     # protocol against the real API, ~40s, no browser
tomoshibi recipes verify     # replay recipe fixtures offline
```

The protocol test builds the Go binaries itself and plays a scripted extension
over the real WebSocket. The full-stack test launches this extension in a real
browser, pairs it through the popup, and drives it through a fixture site
(`fixtures/` holds the two pages the scripted extension serves):

```bash
bun test/e2e-extension.ts    # ~60s; see the note below about which Chrome
```

That test needs a Chromium that honours `--load-extension`. Branded Google
Chrome 137+ ignores it (and its CDP replacement, `Extensions.loadUnpacked`,
registers the extension without ever starting its service worker — every
`chrome-extension://` page answers `ERR_BLOCKED_BY_CLIENT`). Install Chrome for
Testing or Chromium and point `CHROME_BIN` at it, or let the test find the copy
Playwright already keeps in `~/.cache/ms-playwright`. It also needs a Go
toolchain, and it uses ports 7439, 7445 and 9444 plus `/tmp/tomoshibi-e2e`.

## License

PolyForm Noncommercial 1.0.0 — see [LICENSE](./LICENSE). Read it, run it, fork
it for yourself; do not sell it. Everything site-specific lives in recipe packs,
not here.

This directory is the one part of the `tomoshibi` repo that is **not** MIT: the
rest of the repository is MIT, and this licence applies to what is under
`packages/bridge/`.
