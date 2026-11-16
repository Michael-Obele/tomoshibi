# Using the Tomoshibi Bridge

Everything about running the extension, in one place: installing it, pairing,
the popup, every request field, every error code, and what to do when it says no.

If you only read one section, read [Your first fetch](#your-first-fetch).

## Contents

1. [What it is, and when you want it](#what-it-is-and-when-you-want-it)
2. [Install](#install)
3. [Pair](#pair)
4. [The popup, control by control](#the-popup-control-by-control)
5. [The toolbar badge](#the-toolbar-badge)
6. [Your first fetch](#your-first-fetch)
7. [The allowlist](#the-allowlist)
8. [`transport`: who answers the request](#transport-who-answers-the-request)
9. [What happens during a fetch](#what-happens-during-a-fetch)
10. [Pausing, and handing back to you](#pausing-and-handing-back-to-you)
11. [The extraction built-ins](#the-extraction-built-ins)
12. [Screenshots](#screenshots)
13. [Recipes: when you actually need one](#recipes-when-you-actually-need-one)
14. [Search and research through your browser](#search-and-research-through-your-browser)
15. [From an agent](#from-an-agent)
16. [Security and privacy](#security-and-privacy)
17. [Limits](#limits)
18. [Troubleshooting](#troubleshooting)
19. [Files, environment and commands](#files-environment-and-commands)
20. [Uninstall](#uninstall)
21. [FAQ](#faq)

---

## What it is, and when you want it

Tomoshibi fetches as a stranger: no cookies, a datacenter address, a Go TLS stack
wearing a Chrome handshake. Sites that know you refuse it — and the ones that
don't still hand back a login wall.

The bridge fixes that by not being a scraper at all. It is a browser extension
that opens one loopback socket to your local Tomoshibi and, when asked, opens a
real tab in the browser **you are already signed into**, reads it, and hands the
result back.

Reach for it when:

- the page is behind a login you already have,
- the site blocks datacenter or headless traffic,
- the page only works with your session (a dashboard, a subscription, a forum),
- a search engine refuses Tomoshibi's address — which is most of them
  (see [Search](#search-and-research-through-your-browser)).

Do **not** reach for it for ordinary public pages. `static`/`dynamic` is faster,
lighter, and doesn't open a window on your screen.

## Install

You need Tomoshibi running locally, and an extension build.

### 1. Tomoshibi

```bash
docker compose up -d                       # from the repo root
# or
go run ./packages/api/cmd/api             # needs Go 1.25+ and Chromium
```

The bridge is built into the API — nothing extra to enable. It starts a
loopback listener and prints a pairing code at startup.

### 2. The extension

**Chrome or Edge** — download `tomoshibibridge-<version>-chrome.zip` from the
[latest `bridge-v*` release](https://github.com/Michael-Obele/tomoshibi/releases),
unzip it, then:

1. open `chrome://extensions`
2. turn on **Developer mode** (top right)
3. **Load unpacked** → pick the unzipped directory

Chrome cannot install a `.zip` directly; the unzip step is not optional.

**Firefox** — download `tomoshibibridge-<version>-firefox.zip`, then
`about:debugging` → **This Firefox** → **Load Temporary Add-on** → pick the zip.
Firefox drops temporary add-ons when it restarts, so this is a per-session
install by design.

**From source** — in `packages/bridge`:

```bash
bun install && bun run zip      # .output/*-chrome.zip and *-firefox.zip
bun run verify:build            # assert the build is the one we meant to ship
```

`bun run build` / `bun run build:firefox` produce unpacked directories
(`.output/chrome-mv3`, `.output/firefox-mv3`) if you'd rather load those.

### What "loaded" looks like

The toolbar shows the Tomoshibi puzzle piece (there is no icon yet) with a badge.
A fresh install shows a grey/red **`off`** badge until Tomoshibi is running, then
amber **`pair`**.

## Pair

Pairing is one-time per browser, and it is mutual: the extension also verifies
that whatever is listening on the port really holds the key from pairing.

```bash
tomoshibi bridge code        # 6 digits, also printed in the API's startup log
```

Paste them into the popup's **6-digit code** box and press **Pair this browser**.

Facts worth knowing:

- The code **rotates every 10 minutes**, and again after every successful pair —
  a code works exactly once.
- **Ten wrong guesses burn it.** Ask for a new one.
- The server stores only a SHA-256 of each token; the plaintext token exists once,
  in the reply the extension saves. It goes out only after the server proves it
  holds the pairing key, so a process that squats on the port gets neither.
- `tomoshibi bridge status` lists paired browsers with last-seen times.
- `tomoshibi bridge unpair` revokes them (or `unpair <name>` for one). A running
  server re-reads the state file, so the revocation lands on a live socket.

**Re-pairing.** If you delete `~/.config/tomoshi/bridge.json`, reinstall
Tomoshibi, or move to a machine with fresh state, the old token stops working and
the popup shows `err` with an explanation. Delete the pairing in the popup
(**Forget**) and pair again. **Forget** also clears the extension's local token,
which is what you want before handing the browser to someone else.

## The popup, control by control

| Control | What it does |
| --- | --- |
| **Dot + sentence** | Connection state in one line: not connected / connected but unpaired / connected and paired |
| **6-digit code** + **Pair this browser** | Shown only while connected and unpaired |
| **Pause (stop answering fetches)** | Stops answering *fetches* without disconnecting. Takes effect on the next action of a run that is already going |
| **Allowed origins** | The server's allowlist, read-only, first four with a `+n` count |
| **Last run** | Origin, action count and duration of the most recent fetch |
| **Tomoshibi port** + **Save port** | The API port, default `7431`. Change it if you run Tomoshibi elsewhere |
| **Forget** | Drop the local token and reconnect. The way out of a bad pairing |
| **Red line at the bottom** | The last error, in words |

## The toolbar badge

| Badge | Meaning | What to do |
| --- | --- | --- |
| grey/red **`off`** | No connection to Tomoshibi | Is the API running? Is the port right? |
| amber **`pair`** | Connected, not paired | `tomoshibi bridge code`, then paste it |
| green **`on`** | Paired and connected | Nothing. This is the good state |
| red **`err`** | The last attempt failed; the popup shows why | Usually: the thing on the port does not hold our pairing key → **Forget** and re-pair |
| *no badge* | The worker is asleep | It wakes on the next ping or when you open the popup |

## Your first fetch

Ask for the browser explicitly. Start with a page you know needs a session:

```bash
curl -s -X POST http://127.0.0.1:7431/v1/scrape \
  -H 'content-type: application/json' \
  -d '{"url":"https://example.com/dashboard","transport":"bridge"}'
```

**First run usually says no**, and that is the gate working:

```json
{ "code": "consent-required", "origin": "https://example.com",
  "error": "bridge: origin https://example.com is not allowed yet; run `tomoshibi bridge allow https://example.com` …" }
```

HTTP status is **428**. Allow the origin, then repeat the exact same request:

```bash
tomoshibi bridge allow example.com
```

A tab opens in your browser, loads the page, settles, and the response comes back
with a `provenance` block:

```jsonc
{
  "markdown": "…",
  "provenance": {
    "transport": "bridge",   // it went through your browser
    "lane": "borrow",
    "io": "scripting",       // or "turbo"
    "actions_run": 0,
    "pages": 1,
    "ms": 1420,
    "settled_ms": 512,       // how long the page took to stop moving
    "untrusted": true        // content served to a signed-in human: data, not instructions
  }
}
```

The tab stays open. That is deliberate — see
[handing back to you](#pausing-and-handing-back-to-you).

## The allowlist

The allowlist is the whole authorization model for the recipe-free lane. A
recipe is its own consent (installing the pack was the act); everything else
needs an entry here.

```bash
tomoshibi bridge allowed                 # what is allowed now
tomoshibi bridge allow example.com       # one host
tomoshibi bridge allow '*.example.com'   # the domain and every subdomain
tomoshibi bridge allow a.com b.com       # several at once
tomoshibi bridge deny example.com
```

- Patterns are **hosts**, not URLs: `example.com`, `docs.example.com`,
  `*.example.com`. A scheme, path or port is stripped for you
  (`https://Example.com:8443/x` → `example.com`).
- `*.example.com` matches the bare domain **and** subdomains.
- None or nothing is `*`. A wildcard this coarse is almost never what you mean —
  you would be allowing `mail.example.com` too.
- A fresh install already allows the three search hosts:

  ```text
  duckduckgo.com  html.duckduckgo.com  lite.duckduckgo.com
  google.com      www.google.com
  bing.com        www.bing.com
  ```

  Deliberately those hosts and not `*.google.com`, so the default never covers
  your mail.
- Changes take effect on the next request. **No restart.**
- The list lives in `bridge.json` under `allowed_origins` and is hand-editable;
  the running server notices the file changed.

Refusing an origin nobody allowed is not a bug and not an error you should retry
blindly — it is the one thing standing between a local API and remote control of
your signed-in browser.

## `transport`: who answers the request

| Value | Meaning |
| --- | --- |
| `anonymous` (default) | Tomoshibi's own address, no cookies. Unchanged behaviour |
| `bridge` | Your browser. Needs a paired extension and an allowed origin. No recipe |
| `auto` | Try `static`, then `dynamic`; escalate into your browser **only** if the answer was a block or a shell, and only for an allowed origin |

`auto` never turns a working anonymous fetch into a refusal: if the origin is not
allowed, it simply stays anonymous and returns what it got.

`mode: "static"` + `transport: "bridge"` is a contradiction and is refused rather
than silently resolved.

`transport` is orthogonal to `mode`: `mode` says *how to render*, `transport`
says *who is asking*.

## What happens during a fetch

1. **The gate.** Origin in the allowlist (or a recipe covers it)? If not, `428`.
2. **The budget.** Under `BRIDGE_PAGES_PER_HOUR` (600)? If not, refused.
3. **The pace floor.** Two seconds since the last run against the same recipe or
   origin. Simultaneous callers queue instead of arriving together.
4. **The tab.** One pinned tab per origin, created on first use and reused. It is
   visible on purpose — you are the override.
5. **Actions**, if you asked for any, in order. Failed actions are advisory: one
   dead click does not lose an otherwise readable page.
6. **`scroll_collect`**, if you asked for it.
7. **`settle`** — until the DOM is quiet, the scroll height is stable, and no
   finite animation is running.
8. **Read** `document.body.innerText` and `document.documentElement.outerHTML`
   (8 MB HTML / 2 MB text caps).
9. **Challenge markers.** If the page matches a known interstitial, the run stops
   as `awaiting-human` and the tab is left for you.
10. **`page_tree` / `motion`**, if you asked for them.
11. **Screenshot**, if you asked for it.
12. The server re-checks the origin and marks the result `untrusted`, never cached.

The built-in markers are: `Just a moment...`, `Enable JavaScript and cookies to
continue`, `Checking your browser before accessing`, `Verifying you are human`,
`Attention Required!`, `cf-challenge-running`. A recipe may add more via
`expect.absent`.

## Pausing, and handing back to you

Two different things, often confused:

**Pause** is yours. Flip the switch in the popup and the bridge stops answering
fetches immediately; a run already going stops before its next action and reports
`paused`. Nothing is lost — resume and retry.

**`awaiting-human`** is the site's. A challenge wall, a consent gate, a 2FA prompt
— the bridge does not solve it. It stops, marks the response `awaiting-human`
(`502`, `code: "blocked"`, `vendor: "bridge-handoff"`), and **leaves the tab open**.
You clear whatever it is, then retry the same request. The tab is already pinned
and already solved, so the retry usually succeeds first time.

The line: the bridge uses access you already have; it never defeats a control you
don't.

## The extraction built-ins

Four fixed functions that run in the page before it is read. A closed vocabulary
on purpose — no caller-supplied JavaScript reaches a signed-in tab.

| Field | What it does | Reach for it when |
| --- | --- | --- |
| `settle` | Waits until the page stops moving: DOM quiet, scroll height stable, no *finite* animation running. Reports `provenance.settled_ms` | Always useful. Infinite spinners are excluded, or nothing would ever settle |
| `scroll_collect` | Scrolls in steps and reports what each round added | Infinite feeds, lazy lists, "load more" pages |
| `page_tree` | The page outline: `dom` (tag, role, path, box, visibility — works on Firefox) or `ax` (the real accessibility tree — Chrome/turbo), or `both` | You need structure, not text: what is a nav, what is a list, what is actually visible |
| `motion` | What is animating (infinite flagged), what is fixed/sticky, which element owns scrolling | Before parsing a page of carousels and sticky headers |

`settle` is on by default on the generic path — a fixed sleep is a guess.

```bash
curl -s -X POST http://127.0.0.1:7431/v1/scrape -H 'content-type: application/json' -d '{
  "url": "https://example.com/feed",
  "transport": "bridge",
  "settle": { "quiet_ms": 400, "max_ms": 8000 },
  "scroll_collect": { "max_rounds": 8, "item_selector": "article" },
  "page_tree": "dom",
  "motion": true
}'
```

| Option | Defaults | Notes |
| --- | --- | --- |
| `settle.quiet_ms` / `max_ms` / `poll_ms` | 500 / 10000 / 120 | How long the DOM must be quiet, and the ceiling |
| `scroll_collect.max_rounds` / `quiet_rounds` | 8 / 3 | Stops after N rounds with no growth |
| `scroll_collect.step_px` / `pause_ms` | ~0.9 viewport / 350 | Distance per round, wait after each |
| `scroll_collect.item_selector` | — | The repeated element that defines "an item". The precise growth signal |
| `page_tree_options.max_nodes` / `max_depth` / `min_text_len` | 1200 / 12 / 0 | Caps on the outline |

Responses gain `page_tree`, `motion` and `collected`, and `provenance.settled_ms`.
All optional; ordinary pages come back exactly as before.

## Screenshots

```jsonc
{ "screenshot": true, "screenshot_opts": { "full_page": true } }
```

| Layer | Captures |
| --- | --- |
| `scripting` (default) | The **viewport only** — `tabs.captureVisibleTab` has no beyond-viewport mode. `screenshot_opts.full_page` is reported honestly as `false` |
| `turbo` (recipe with `io: turbo`) | Full page, `captureBeyondViewport` |

A scripting-layer screenshot is skipped (and says so in `provenance.actions_failed`)
if another tab becomes active first — the alternative is screenshotting someone
else's page.

## Recipes: when you actually need one

**Most uses need none.** A recipe is the specialization for a site the generic
driver cannot handle:

- it needs its own navigation steps (click this, type that, press Enter),
- it needs its own extraction schema,
- it needs its own pace,
- it needs `io: turbo` (trusted CDP input, full-page screenshots, `evaluate`,
  `wait_for_function`).

```bash
tomoshibi recipes add <dir|git-url>   # verify hashes, install
tomoshibi recipes list                # what is installed
tomoshibi recipes verify <pack>       # replay saved fixtures, no network
tomoshibi recipes remove <pack>
```

A pack that no longer parses fails `verify` before it ever drives a browser —
that is what stops silent garbage when a site redesigns.

Two rules worth remembering:

- **A recipe must cover the host it is applied to.** An explicit recipe id naming
  a foreign host is an error, not a shortcut.
- **A recipe is its own consent.** You do not also need an allowlist entry.

`io: turbo` requires Chrome: the Firefox build has no `debugger` permission and
says so instead of falling back silently.

Pack format, the `expect` contract and the full schema: see the
[API README's recipes section](../api/README.md#recipes) (from the repo root).

## Search and research through your browser

The same rung turns search engines that block a datacenter address back on. This
is the reason the bridge exists for many people: DuckDuckGo answered **30 times
and refused 40** from this machine's own address in one week.

```bash
# one engine, chosen per request
curl -s -X POST http://127.0.0.1:7431/v1/search -H 'content-type: application/json' \
  -d '{"query":"svelte 5 runes","engine":"browser:ddg"}'

# the query AND the pages behind it
curl -s -X POST http://127.0.0.1:7431/v1/research -H 'content-type: application/json' \
  -d '{"query":"svelte 5 runes","engine":"browser:google","pages":5,"summary":true}'
```

- `engine` accepts `browser:ddg`, `browser:google`, `browser:bing`. Omit it for
  the configured chain, which puts the free engines first and the paid API last.
- `research` returns `results`, `pages`, `failures`, `elapsed_ms` and
  `paid_api_used` — so you can see what a query actually cost.
- An engine challenge is `502` with `code: "awaiting-human"` on both endpoints.
  Never a 500, never an empty list pretending the query had no answers.
- Google serves a consent wall to a profile that has not answered it. The bridge
  clicks the explicit **Accept all** control once, and the cookie does the rest.

## From an agent

The MCP exposes all of this as tool parameters:

```jsonc
// tomoshi_extract
{
  "action": "scrape",
  "url": "https://example.com/dashboard",
  "transport": "bridge",     // or "auto"
  "settle": true,
  "page_tree": "dom",
  "motion": true
}

// tomoshi_extract — one query, then the pages
{ "action": "research", "query": "svelte 5 runes", "engine": "browser:ddg", "pages": 5 }

// tomoshi_discover — engine switching
{ "action": "search", "query": "…", "engine": "browser:bing" }
```

`settle` and `scroll_collect` accept `true` as shorthand for the defaults. Full
tool reference: [packages/mcp/README.md](../../mcp/README.md).

## Security and privacy

What the extension **cannot** do — enforced in the manifest and asserted by tests
in CI:

- **No cookie access.** `cookies`, `webRequest`, `nativeMessaging`,
  `externally_connectable`, `history` and `bookmarks` are not in either manifest.
- **No credential typing, ever.** Password, passcode, one-time-code, card fields,
  and anything named `pwd` / `pin` / `otp` / `2fa` / `cvv` are refused at the
  field level, in both layers.
- **No challenge solving**, no proxy rotation, no account pools.
- **No reading tabs it wasn't asked about.** One pinned tab per origin, opened on
  demand.
- **No remote code.** Everything it runs ships in the zip.

How the connection is protected:

- The socket is **loopback-only**, and Tomoshibi refuses any `http(s)` origin — a
  web page in your own browser is a loopback peer too.
- **Mutual auth.** The extension releases its token only to a process that proves
  it holds the key from pairing. A squatter on the port gets nothing.
- **Origin drift is refused.** If the page navigates somewhere other than the
  origin you asked for, the run stops rather than reading another site with your
  session.
- **Every fetch is logged** to `~/.config/tomoshi/access.log.jsonl` — recipe,
  lane, origin, result.

Your session never leaves the machine. Nothing is exported, synced or tunneled.

## Limits

| Limit | Value | Override |
| --- | --- | --- |
| Pages per hour, borrow lane | 600 | `BRIDGE_PAGES_PER_HOUR` (`0` disables) |
| Actions per page | 64 | — |
| HTML read | 8 MB | — |
| Text read | 2 MB | — |
| One frame | 24 MiB | protocol constant |
| Pace floor between runs | 2 s per recipe/origin | recipe `access.pace.min_interval_ms` |
| Per-page timeout | 60 s | recipe `request.timeout_ms` |

The budget exists because a research loop with a bug should cost a refusal, not a
hundred tabs on a real session.

## Troubleshooting

### By what you see

| Symptom | Cause | Fix |
| --- | --- | --- |
| Badge `off` | No connection | Is the API running? Is the port in the popup right? Is another Tomoshibi holding the port? |
| Badge `pair`, no pair box | Not connected yet | The pair box appears only once the socket is up |
| Badge `err`, "it does not hold our pairing key" | The server was reinstalled or its state deleted | Popup → **Forget** → `tomoshibi bridge code` → pair again |
| The code is rejected | Codes rotate every 10 min, work once, and burn after 10 wrong guesses | Get a fresh one |
| A tab opens and the fetch fails | Expected on the first crack at a new origin | Check the error code below |
| Nothing happens at all | The worker is asleep, or the extension was reloaded | Open the popup (wakes it), or toggle the extension off/on |
| Chrome refuses to load the folder | You picked the source directory | Pick `.output/chrome-mv3` (or the unzipped release folder), not the package folder |

### By error code

| Code | HTTP | Meaning | Fix |
| --- | --- | --- | --- |
| `consent-required` | 428 | Origin is not in the allowlist | `tomoshibi bridge allow <host>` |
| `bridge-offline` | 502 | No browser connected | Start Chrome with the extension loaded; check the badge |
| `bridge-unpaired` | 502 | Connected but not paired | Pair in the popup |
| `bridge-timeout` | 502 | The browser did not answer in time | The tab may be stuck loading — check it |
| `bridge-disconnected` | 502 | The socket dropped mid-fetch | Retry; usually a browser restart |
| `paused` | 502 | The pause switch is on | Flip it off in the popup |
| `blocked` (with `vendor`) | 502 | A challenge page was served | Clear it in the open tab, then retry |
| `blocked` + `vendor: bridge-handoff` | 502 | `awaiting-human`: a marker matched | Same — solve it in the tab, retry |
| `awaiting-human` | 502 | An engine served its own interstitial | Clear it in the bridge tab, retry the query |

`provenance.actions_failed` names every action the browser refused, with the
reason. A page that came back does not tell you that half a recipe was skipped;
that field does.

### Turning up the detail

```bash
tomoshibi bridge status        # port, paired devices, last seen
tomoshibi access log -n 20     # the last 20 fetches: recipe, lane, origin, result
```

## Files, environment and commands

| What | Where |
| --- | --- |
| State: pairing code, port, device token hashes, server key, allowlist | `~/.config/tomoshi/bridge.json` (mode `0600`) |
| Audit log: one line per fetch | `~/.config/tomoshi/access.log.jsonl` |
| The same dir, elsewhere | `BRIDGE_STATE_DIR` |

```
tomoshibi bridge code                 print the current pairing code and port
tomoshibi bridge status               paired devices and last-seen times
tomoshibi bridge unpair [name|all]    revoke a paired browser (default: all)
tomoshibi bridge allow  <host...>     add to the recipe-free allowlist
tomoshibi bridge deny   <host...>     remove from it
tomoshibi bridge allowed              print it
tomoshibi recipes add <dir|git-url>   verify and install a pack
tomoshibi recipes list                installed packs and their recipes
tomoshibi recipes verify [id]         offline fixture check (no network)
tomoshibi recipes remove <pack>       uninstall a pack (name@sha)
tomoshibi access log [-n N]           the last N access-log lines (default 20)
```

Environment variables that matter to this feature:

| Variable | Default | Effect |
| --- | --- | --- |
| `BRIDGE_STATE_DIR` | `~/.config/tomoshi` | Where state and the audit log live |
| `BRIDGE_PAGES_PER_HOUR` | `600` | Borrow-lane page budget; `0` disables |
| `SERVER_PORT` | `7431` | The bridge listener is mounted on the API server itself, at `/bridge` — there is no separate bridge port |

## Uninstall

1. `tomoshibi bridge unpair` — revoke every browser. Takes effect on live sockets.
2. Remove the extension: `chrome://extensions` → Remove, or `about:debugging` →
   Remove.
3. Optionally `rm ~/.config/tomoshi/bridge.json` and `access.log.jsonl`.

Pairing is one browser at a time; uninstalling from one Chrome does not affect
another.

## FAQ

**Do I need a recipe?** No. `transport: "bridge"` is enough once the origin is
allowed.

**Does it open a window every time?** Yes — one pinned tab per origin, reused
across fetches. Visible on purpose: you are the override.

**Can it run unattended?** Not for the borrow lane. Crawl and monitor deliberately
refuse it: a crawl follows links nobody chose (one could be a sign-out link) and a
monitor repeats unattended.

**Will it get my account banned?** It uses your real session at a human-ish pace
with a page budget. That is the least suspicious option available — but it is
still automation, and the site's terms are the site's terms.

**Why does Google ask me to accept cookies once?** A fresh profile hits Google's
consent wall. The bridge clicks the explicit **Accept all** control a person would
click, once; the cookie then persists in your profile.

**Does the browser have to stay open?** Yes. Whole point.

**Can I use it from another machine?** The socket is loopback-only and refuses
web origins. Remote use means a tunnel to Tomoshibi, not to the browser — and that
is not built.

**Firefox support?** Yes, scripting layer only. `io: turbo` needs Chrome's
`debugger` API.
