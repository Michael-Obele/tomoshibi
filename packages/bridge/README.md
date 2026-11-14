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
2. Load this folder as an unpacked extension: `chrome://extensions` →
   Developer mode → **Load unpacked** → pick this directory.
3. Run `tomoshibi bridge code` (or read the startup log) and type the 6-digit
   code into the extension popup. One pairing per browser; the token is stored
   locally and revoked with `tomoshibi bridge unpair` (or by deleting
   `~/.config/tomoshi/bridge.json`, which the running server notices).
4. Install a recipe pack and scrape with it: `POST /v1/scrape {"url": ...,
   "recipe": "<id>"}`.

The popup also holds the API port (default `7431`), the pairing code box, and a
pause switch. The toolbar badge says which state it is in: green `on` means
paired and connected, amber `pair` means connected but unpaired, grey-red `off`
means no connection, red `err` means the last attempt failed (the popup shows
why).

The manifest asks for `host_permissions: ["<all_urls>"]`, because a recipe may
name any host. Nothing is read until a fetch names a URL.

Firefox: copy `manifest.firefox.json` over `manifest.json` before loading
(`about:debugging` → This Firefox → Load Temporary Add-on). The Firefox
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
  `io: turbo` recipes, you can delete it and remove `runTurbo` from
  `src/background.js`.

## Development

Zero build step. Edit the files and reload the extension. There is no icon, so
the toolbar shows the default puzzle piece until one is added.

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

PolyForm Noncommercial 1.0.0. Read it, run it, fork it for yourself; do not
sell it. Everything site-specific lives in recipe packs, not here.
