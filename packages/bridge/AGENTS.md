# Tomoshibi Bridge — the agent contract

`packages/bridge/` is the browser half of Tomoshibi's `borrow` lane: a zero-build
MV3 extension that lets a local Tomoshibi API drive the browser you are already
signed into, over one loopback WebSocket. Read this before editing.

It lives inside the `tomoshibi` monorepo, next to the server it talks to. It was
its own repo until 2026-11-14; the only thing that had made it separate was a
different licence, and a licence applies to a directory just as well as to a
repo (D29). Two consequences:

- The repo root's rules still apply. Read `AGENTS.md` at the repo root and
  `packages/api/AGENTS.md` if it exists; this file only covers what is specific
  to the extension.
- `packages/bridge/` is the one non-MIT directory in the repo. It is PolyForm
  Noncommercial 1.0.0 (`LICENSE` here); everything around it is MIT. Do not
  relicense, and do not copy extension code into a MIT package.

- Product plan of record: `AI-Plans/plans/lanyard/` (D19–D27 are what shipped,
  D29 is the move into this repo).
- Target shape: `docs/product.md` — WXT + Svelte 5. Read "State of the repo"
  before you plan any change.

## State of the repo (2026-11-14, later)

**The migration landed.** WXT builds the extension, the background is
TypeScript, and the popup is Svelte 5. D20 (zero-build) is superseded by D34.

- `entrypoints/background/index.ts` — the service worker and the protocol.
- `entrypoints/popup/{index.html,main.ts,App.svelte}` — the only UI.
- `utils/protocol.ts` — the wire constants and types, pinned against
  `packages/api/internal/bridge/protocol.go`.
- `utils/guard.ts` — the pure refusals (credential field, origin drift, stop
  reason). No browser API, so each one is unit-testable on its own.
- `utils/page.ts` — the functions injected into a tab. Each MUST be
  self-contained (no imports, no closures): `scripting.executeScript`
  serialises them with `toString()`, so a transpiler helper would be a dangling
  reference in the page. `vite.build.target = 'es2022'` in `wxt.config.ts` is
  what keeps async/await native; do not lower it.

`docs/product.md` still owns the target shape (side panel, options). The popup
is done; steps 5–8 of its migration order remain.

## Non-negotiables

Each rule is enforced by code or a test. A failing check is never fixed by
relaxing the rule.

| # | Rule | Where it lives |
| - | ---- | -------------- |
| 1 | No `cookies`, `webRequest`, `nativeMessaging`, `externally_connectable`, `history` or `bookmarks` in either manifest | `test/e2e-protocol.ts` asserts it over both manifests |
| 2 | Never type into a credential field | `isCredentialField` in `utils/guard.ts`, called by both IO layers; `test/e2e-extension.ts` proves the refusal |
| 3 | Never solve a challenge. A marker hit is `awaiting-human` (tab left open), never content | `detectMarkers` in `utils/guard.ts` + `BUILTIN_MARKERS` in `utils/protocol.ts`; recipe `expect.absent` turns it into `502 blocked` |
| 4 | Never drive or read an origin the fetch did not name | `originDrift` and `assertSameOrigin`, checked between actions and again at the end |
| 5 | Nothing runs for an unverified peer; the token leaves only after the server proves it holds the pairing key | `onChallenge` in `entrypoints/background/index.ts` (D19) |
| 6 | No half-migration: the spike stays zero-build until a migration step lands whole | D20 binds `src/*.js` today; `docs/product.md` replaces it |
| 7 | Loopback only: dials `ws://127.0.0.1:<port>/bridge`, never listens, never tunnels | D22 and the server's origin check |
| 8 | Keep the caps composed: 8 MB HTML + 2 MB text + a ≤8 MB base64 PNG must all fit one 24 MiB frame | constants in `entrypoints/background/index.ts` and `internal/bridge/protocol.go` |

## Repo map

| Path | What it is |
| ---- | ---------- |
| `entrypoints/background/index.ts` | Service worker: socket, handshake, popup API, both IO layers, tab pinning, the named built-ins |
| `entrypoints/popup/App.svelte` | The only UI: state dot, pairing code, pause, allowlist summary, last run, port, forget |
| `utils/protocol.ts` | Protocol constant, default port, challenge markers, wire types |
| `utils/guard.ts` | Pure refusals: credential field, origin drift, stop reason, host matching |
| `utils/page.ts` | Functions injected into a tab (readPage, click, type, press, settle, scroll_collect, page_tree, motion) |
| `wxt.config.ts` | Both manifests from one file; `debugger` is Chrome-only; `build.target = es2022` is load-bearing |
| `test/cdp.ts` | Which Chrome, the CDP client, extension-id derivation — shared by B and C |
| `test/harness.ts` | Fixture site, bin build, API spawn, recipe pack written on the fly, `check()` tally |
| `test/e2e-protocol.ts` | E2E-A: protocol against the real Go API with a scripted extension |
| `test/e2e-extension.ts` | E2E-B: this extension in real Chrome, paired through the popup. 19 checks |
| `test/live-research.ts` | E2E-C: real engines through the real browser — ddg, google, bing, then a research fan-out. Needs the internet |
| `scripts/verify-build.mjs` | MV3 on both targets, version matches `package.json`, no forbidden permission, both zips exist. Run by CI, runnable locally |
| `test/unit/` | `bun test`: the guards, and settle/collect/tree/motion against happy-dom |
| `fixtures/` | The two pages the scripted extension serves in E2E-A |

## Commands

Everything below was run on this machine and passed. Bun 1.4.4, Go 1.25.4,
WXT 0.21.5, Vite 8.

```bash
bun run check              # wxt prepare + tsc --noEmit + unit tests
bun run test               # check, then E2E-A and E2E-B
bun run build              # .output/chrome-mv3
bun run build:firefox      # .output/firefox-mv3
bun run zip                # both targets -> .output/*.zip (the installable artifact)
bun run verify:build       # assert the build is the one we meant to ship

bun test/e2e-protocol.ts   # 25 checks, ~40 s. Builds the Go binaries and plays
                           # a scripted extension. No browser, no display.
bun test/e2e-extension.ts  # 19 checks, ~90 s. The real extension in Chrome for
                           # Testing, paired through the popup.
bun test/live-research.ts  # real ddg/google/bing plus a research fan-out.
                           # Needs the internet; not part of `bun run test`.
```

These are plain scripts with their own `check()` tally, not `bun test` suites.
`test/unit/` is the exception: those are real `bun test` files with a happy-dom
preload from `bunfig.toml`.

Manual run:

```bash
# 1. API (writes the pairing state and logs the code on startup)
cd packages/api && go run ./cmd/api

# 2. Load packages/bridge unpacked: chrome://extensions → Developer mode →
#    Load unpacked → pick packages/bridge (not the repo root).

# 3. Pair
cd packages/api
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

Protocol 1 is pinned in two halves of this repo and must be bumped together:
`PROTOCOL` in `utils/protocol.ts` and `ProtocolVersion` in
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
| A read cap or frame size | `entrypoints/background/index.ts` constants and `MaxMessageBytes`/`readPageExpression` on the server side |
| A popup state | `entrypoints/popup/App.svelte` render, `badge()` calls, and the state list in `docs/product.md` |
| Anything user-visible | the README's install and security sections |

## Gotchas that cost an hour

- Branded Chrome 137+ ignores `--load-extension`, and `Extensions.loadUnpacked`
  registers the extension without starting the worker. Use Chrome for Testing
  or Chromium (D21). Users are unaffected: they load unpacked by hand.
- `test/harness.ts` finds the API package relative to itself
  (`packages/bridge/test` -> `packages/api`), so the suite runs from any clone.
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

`README.md` is the human door and `docs/USAGE.md` is the manual (install → pair →
request fields → error codes → troubleshooting). `AI-Plans/plans/lanyard/bridge.md` holds the
protocol detail and prior art, `decisions.md` holds D19–D27, and `milestones.md`
holds the M1b evidence and the gaps. `docs/product.md` is the product this spike
becomes, including why the extension UI is Svelte and not SvelteKit.
