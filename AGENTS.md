# Tomoshibi Bridge — the agent contract

This repo is the browser half of Tomoshibi's `borrow` lane: a zero-build MV3
extension that lets a local Tomoshibi API drive the browser you are already
signed into, over one loopback WebSocket. Read this before editing; it is the
only agent-instruction file in the repo.

- Product plan of record: `AI-Plans/plans/lanyard/` (D19–D27 are what shipped).
- Target shape: `docs/product.md` — WXT + Svelte 5. Read "State of the repo"
  before you plan any change.
- Licence: PolyForm Noncommercial 1.0.0. Do not relicense.

## State of the repo (2026-11-14)

Two shapes exist; only one is real at a time.

- **Now — the spike.** `src/*.js` is plain MV3: four files, two hand-written
  manifests, no build step (D20). Everything in this file describes it, and both
  suites are green against it.
- **Target — the product.** WXT + Svelte 5 + TypeScript, built to
  `.output/chrome-mv3` and `.output/firefox-mv3`, with a popup, a side panel and
  an options page. Planned in `docs/product.md`.

Do not half-migrate. A change either touches the spike as it is, or executes one
numbered step of the migration plan. Mixing the two makes both suites lie.

## Non-negotiables

Each rule is enforced by code or a test. A failing check is never fixed by
relaxing the rule.

| # | Rule | Where it lives |
| - | ---- | -------------- |
| 1 | No `cookies`, `webRequest`, `nativeMessaging`, `externally_connectable`, `history` or `bookmarks` in either manifest | `test/e2e-protocol.ts` asserts it over both manifests |
| 2 | Never type into a credential field | `isCredentialField` in `src/background.js`, called by both IO layers; `test/e2e-extension.ts` proves the refusal |
| 3 | Never solve a challenge. A marker hit is `awaiting-human` (tab left open), never content | `detectMarkers` + `BUILTIN_MARKERS` in `src/lib.js`; recipe `expect.absent` turns it into `502 blocked` |
| 4 | Never drive or read an origin the fetch did not name | `originDrift` and `assertSameOrigin`, checked between actions and again at the end |
| 5 | Nothing runs for an unverified peer; the token leaves only after the server proves it holds the pairing key | `onChallenge` in `src/background.js` (D19) |
| 6 | No half-migration: the spike stays zero-build until a migration step lands whole | D20 binds `src/*.js` today; `docs/product.md` replaces it |
| 7 | Loopback only: dials `ws://127.0.0.1:<port>/bridge`, never listens, never tunnels | D22 and the server's origin check |
| 8 | Keep the caps composed: 8 MB HTML + 2 MB text + a ≤8 MB base64 PNG must all fit one 24 MiB frame | constants in `src/background.js` and `internal/bridge/protocol.go` |

## Repo map

| Path | What it is |
| ---- | ---------- |
| `src/background.js` | Service worker: socket, handshake, popup API, both IO layers, tab pinning |
| `src/lib.js` | Protocol constant, default port, challenge markers, `getSettings` |
| `src/popup.{html,js}` | The only UI: state dot, pairing code, pause, port, forget |
| `manifest.json` | Chrome MV3. `debugger` lives here only |
| `manifest.firefox.json` | Firefox MV3: no `debugger`, `background.scripts` |
| `test/harness.ts` | Fixture site, bin build, API spawn, recipe pack written on the fly, `check()` tally |
| `test/e2e-protocol.ts` | E2E-A: protocol against the real Go API with a scripted extension. 25 checks |
| `test/e2e-extension.ts` | E2E-B: this extension in real Chrome, paired through the popup. 19 checks |
| `fixtures/` | The two pages the scripted extension serves in E2E-A |

## Commands

Everything below was run on this machine and passed. Bun 1.4.4, Go 1.25.4.

```bash
bun test/e2e-protocol.ts     # 25 checks, ~40 s. Builds the Go binaries and plays
                             # a scripted extension. No browser, no display.
bun test/e2e-extension.ts    # 19 checks, ~90 s. Real extension in Chrome for
                             # Testing. Needs Go and a Chromium that honours
                             # --load-extension.
```

These are plain scripts with their own `check()` tally, not `bun test` unit
tests. Run the files; do not look for a test config.

Manual run:

```bash
# 1. API (writes the pairing state and logs the code on startup)
cd ~/Documents/GitHub/tomoshibi/packages/api && go run ./cmd/api

# 2. Load this folder unpacked: chrome://extensions → Developer mode →
#    Load unpacked → pick the repo root.

# 3. Pair
cd ~/Documents/GitHub/tomoshibi/packages/api
go run ./cmd/tomoshibi bridge code     # 6 digits, 10 min TTL
go run ./cmd/tomoshibi bridge status   # port + paired devices
go run ./cmd/tomoshibi bridge unpair   # revokes a live socket
go run ./cmd/tomoshibi access log -n 5 # the audit trail

# 4. Scrape through it
curl -s -X POST http://127.0.0.1:7431/v1/scrape \
  -H 'content-type: application/json' -H 'x-api-key: <key>' \
  -d '{"url":"https://example.com","recipe":"<id>"}'
```

There is no build, no lint and no typecheck step in the spike. Do not invent
one. The WXT migration adds them; see step 1 in `docs/product.md`.

Test environment, all overridable: state `/tmp/tomoshibi-e2e` (via
`BRIDGE_STATE_DIR`), API port 7439, fixture site 7445, CDP 9444,
`CHROME_BIN` for the browser. The harness finds Chrome for Testing in
`~/.cache/ms-playwright/chromium-*` when `CHROME_BIN` is unset.

## The seam

Protocol 1 is pinned in two repos and must be bumped together:
`PROTOCOL` in `src/lib.js` and `ProtocolVersion` in
`tomoshibi/packages/api/internal/bridge/protocol.go`. A mismatch is refused,
not guessed.

- Frames: `hello` → `challenge` → `auth` → `status`; `pair` → `pair.ok`;
  `pair.required`; `fetch` → `fetch.result`; `cancel`, `ping`/`pong`, `error`.
- `hello` never carries the token. That is the whole of D19; do not "simplify" it.
- Action types the server may send: `wait_selector`, `wait_ms`,
  `wait_for_function`, `scroll_to_bottom`, `scroll_by`, `scroll_down`, `click`,
  `type`, `press`, `navigate`, `evaluate`. The scripting layer refuses the last
  two with a readable reason; turbo runs them.
- The server needs `final_url`, `title`, `text`, `html`, `html_truncated`,
  `http_status`, `screenshot_b64`, `status`, `markers`, `actions_run`, `ms`.
- Failure codes an agent branches on: `bridge-offline`, `bridge-unpaired`,
  `bridge-timeout`, `bridge-disconnected`, `paused`, `blocked`.

## If you change X, also change Y

| Change | Also change |
| ------ | ----------- |
| An action type | both `runScriptingAction` and `runTurboAction`, the server's `bridge.Action`, the recipe schema in tomoshibi, and one check in E2E-B |
| A frame or a field the server reads | bump protocol in both repos, then E2E-A |
| A permission | both manifests, the permission assertion in E2E-A, and the README's "will not do" list |
| A read cap or frame size | `src/background.js` constants and `MaxMessageBytes`/`readPageExpression` on the server side |
| A popup state | `src/popup.js` render, `badge()` calls, and the state list in `docs/product.md` |
| Anything user-visible | the README's install and security sections |

## Gotchas that cost an hour

- Branded Chrome 137+ ignores `--load-extension`, and `Extensions.loadUnpacked`
  registers the extension without starting the worker. Use Chrome for Testing
  or Chromium (D21). Users are unaffected: they load unpacked by hand.
- `test/harness.ts` hardcodes the API checkout as
  `$HOME/Documents/GitHub/tomoshibi/packages/api`. A clone elsewhere means
  editing `REPO` in that one file.
- MV3 workers sleep. A 20 s server ping keeps the socket alive; a 30 s alarm
  reconnects when nothing is listening.
- The storage listener would restart the socket when the worker writes its own
  token. `ownToken` exists to swallow that echo. Keep it.
- Every socket handler ignores events from a socket that is no longer `ws`. A
  replaced socket's late `close` must not flip the badge or schedule a reconnect.
- `captureVisibleTab` shoots the active tab of a window. Re-read the active tab
  and skip the screenshot rather than send someone else's page.
- The Firefox build has no `debugger`, so `io: turbo` throws a readable error.
  That is correct; do not fall back to scripting silently.
- The pinned tab is left open on a handoff on purpose (D16). Do not add cleanup.
- The CLI reads the state dir; it does not create it. Before the API has run
  once, `bridge code` and `bridge status` exit 1.
- The pairing code rotates on success, on its 10 min TTL, and after ten wrong
  guesses. A test that hardcodes a code will flake.

## Definition of done

- `bun test/e2e-protocol.ts` and `bun test/e2e-extension.ts` both green, and you
  report the check counts you saw.
- No new permission, dependency, build step or tracked file outside the repo map.
- If an action or frame moved: the tomoshibi counterpart is updated and the
  protocol version bumped in both places.
- The refusal that covers your change still has a test. If it does not, that is
  the missing work, not the feature.

## Where to go deeper

`README.md` is the human door. `AI-Plans/plans/lanyard/bridge.md` holds the
protocol detail and prior art, `decisions.md` holds D19–D27, and `milestones.md`
holds the M1b evidence and the gaps. `docs/product.md` is the product this spike
becomes, including why the extension UI is Svelte and not SvelteKit.
