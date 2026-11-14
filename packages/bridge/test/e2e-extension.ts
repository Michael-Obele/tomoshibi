// E2E-B: the real extension in real Chrome, end to end.
//
// Launches headless Chrome for Testing with the extension loaded, points it
// at the test API through the popup UI, pairs it the same way a person would,
// then asks the API to scrape through the bridge:
//   - scripting recipe: types into a search box, clicks, returns the result
//   - turbo recipe:     same flow through chrome.debugger, full-page screenshot
//   - challenge page:   reported as blocked, never as content
//
// This is the test that proves "the browser you are signed into" works with no
// debug toggle, no CDP port the extension depends on, and no manual steps.

import { spawn } from 'bun';
import { mkdirSync, writeFileSync, readFileSync, existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import {
  resetRunDir, buildBinaries, startFixtureSite, startAPI, writePack, installPack,
  scrape, readState, RUN_DIR, STATE_DIR, SITE_PORT, CDP_PORT, API_PORT, BRIDGE_DIR, check,
  assertAllPassed,
} from './harness';

const EXT_DIR = BRIDGE_DIR;

// WHICH CHROME, AND WHY IT MATTERS
//
// Branded Chrome 137+ ignores --load-extension for automation, and the CDP
// replacement (Extensions.loadUnpacked) only half-works: the extension is
// registered (it even shows up in the profile's Preferences) but Chrome never
// starts its service worker and every chrome-extension:// page answers
// ERR_BLOCKED_BY_CLIENT. Verified on Google Chrome 157.0.8103.50, with and
// without --enable-unsafe-extension-debugging.
//
// Chrome for Testing (or Chromium) still honours --load-extension, and the
// worker comes up on the next tick. Playwright keeps a CfT build in the cache,
// which is the build this test uses unless CHROME_BIN says otherwise.
function resolveChrome(): string {
  if (process.env.CHROME_BIN && existsSync(process.env.CHROME_BIN)) return process.env.CHROME_BIN;
  const root = join(process.env.HOME!, '.cache/ms-playwright');
  if (existsSync(root)) {
    const builds = readdirSync(root).filter((d) => d.startsWith('chromium-')).sort().reverse();
    for (const d of builds) {
      for (const layout of ['chrome-linux64', 'chrome-linux']) {
        const bin = join(root, d, layout, 'chrome');
        if (existsSync(bin)) return bin;
      }
    }
  }
  throw new Error('no Chrome for Testing build found (set CHROME_BIN to a Chromium/CfT binary; branded Chrome will not load the extension)');
}

const CHROME_BIN = resolveChrome();

// ------------------------------------------------------------------ CDP glue

type Pend = { resolve: (v: any) => void; reject: (e: Error) => void };

class CDP {
  private ws: WebSocket;
  private seq = 0;
  private pending = new Map<number, Pend>();
  private sessions = new Map<string, (msg: any) => void>();

  private constructor(ws: WebSocket) {
    this.ws = ws;
    ws.onmessage = (ev) => {
      const msg = JSON.parse(String(ev.data));
      if (msg.id) {
        const p = this.pending.get(msg.id);
        if (p) {
          this.pending.delete(msg.id);
          msg.error ? p.reject(new Error(JSON.stringify(msg.error))) : p.resolve(msg.result);
        }
        return;
      }
      if (msg.sessionId) {
        const h = this.sessions.get(msg.sessionId);
        if (h) h(msg);
      }
    };
  }

  static connect(url: string): Promise<CDP> {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url);
      ws.onopen = () => resolve(new CDP(ws));
      ws.onerror = () => reject(new Error(`CDP connect failed: ${url}`));
    });
  }

  send(method: string, params: Record<string, unknown> = {}, sessionId?: string): Promise<any> {
    const id = ++this.seq;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    });
  }

  /** Attach to a target and return its session id. */
  async attach(targetId: string): Promise<string> {
    const { sessionId } = await this.send('Target.attachToTarget', { targetId, flatten: true });
    return sessionId;
  }

  async eval(sessionId: string, expression: string): Promise<any> {
    const r = await this.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }, sessionId);
    if (r.exceptionDetails) throw new Error(`evaluate: ${r.exceptionDetails.text} ${r.exceptionDetails.exception?.description || ''}`);
    return r.result?.value;
  }

  close() {
    try { this.ws.close(); } catch { /* gone */ }
  }
}

function extensionIdFromPath(path: string): string {
  const hash = new Bun.CryptoHasher('sha256').update(path).digest();
  let id = '';
  for (let i = 0; i < 16; i++) {
    id += String.fromCharCode(97 + (hash[i] >> 4));
    id += String.fromCharCode(97 + (hash[i] & 0xf));
  }
  return id;
}

async function waitFor(fn: () => Promise<boolean> | boolean, timeoutMs: number, what: string): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await fn()) return;
    await Bun.sleep(300);
  }
  throw new Error(`timeout waiting for ${what}`);
}

// -------------------------------------------------------------------- main

console.log('== E2E-B: real extension in real Chrome ==');
resetRunDir();
const site = startFixtureSite();
await buildBinaries();
writePack();
const api = await startAPI();
await installPack();

const chromeDir = join(RUN_DIR, 'chrome');
mkdirSync(chromeDir, { recursive: true });
const chrome = spawn([
  CHROME_BIN,
  '--headless=new',
  '--no-sandbox',
  '--disable-gpu',
  '--disable-dev-shm-usage',
  '--hide-scrollbars',
  '--no-first-run',
  '--no-default-browser-check',
  `--user-data-dir=${chromeDir}`,
  `--remote-debugging-port=${CDP_PORT}`,
  `--disable-extensions-except=${EXT_DIR}`,
  `--load-extension=${EXT_DIR}`,
  // The recipes that are not on localhost own their host names; resolving them
  // here keeps the test on the recipe's declared host instead of special-casing
  // the recipe. Everything still lands on the fixture site.
  `--host-resolver-rules=MAP turbo.test 127.0.0.1,MAP challenge.test 127.0.0.1,MAP link.test 127.0.0.1,MAP login.test 127.0.0.1`,
  // Keeps built-in component extensions (and their workers) out of the way so
  // "a chrome-extension:// worker" unambiguously means ours.
  '--disable-component-extensions-with-background-pages',
  'about:blank',
], { stdout: 'pipe', stderr: 'pipe' });

let cdp: CDP | null = null;
try {
  // 1. Chrome + CDP up.
  await waitFor(async () => {
    try {
      const res = await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`, { signal: AbortSignal.timeout(1000) });
      return res.ok;
    } catch {
      return false;
    }
  }, 20000, 'chrome devtools endpoint');
  const version = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`)).json();
  cdp = await CDP.connect(version.webSocketDebuggerUrl);
  check('chrome launched with the extension', true, `${version.Browser} (${CHROME_BIN})`);

  // 2. Find OUR service worker by its file name (never just "some extension").
  //    The worker may not have started yet, so as a fallback open the popup,
  //    which pings the background and wakes it.
  let swTargetId: string | null = null;
  let extIdReal = '';
  const findSW = async (): Promise<boolean> => {
    const { targetInfos } = await cdp!.send('Target.getTargets');
    const sw = targetInfos.find((t: any) => t.type === 'service_worker' && String(t.url).endsWith('/src/background.js'));
    if (sw) {
      swTargetId = sw.targetId;
      extIdReal = String(sw.url).split('/')[2];
      return true;
    }
    return false;
  };
  if (!(await findSW())) {
    const guessed = extensionIdFromPath(EXT_DIR);
    await cdp.send('Target.createTarget', { url: `chrome-extension://${guessed}/src/popup.html` }).catch(() => {});
    const found = await waitFor(findSW, 15000, 'bridge service worker target').then(() => true).catch(() => false);
    if (!found) {
      const { targetInfos } = await cdp.send('Target.getTargets');
      const workers = targetInfos.filter((t: any) => String(t.url).startsWith('chrome-extension://')).map((t: any) => `${t.type} ${t.url}`);
      throw new Error(`bridge service worker never appeared. Extension workers seen:\n${workers.join('\n') || '(none)'}`);
    }
  }
  check('bridge extension loaded in Chrome', !!extIdReal, extIdReal);

  // 3. Pair through the real popup UI.
  const code = readState().code;
  const popup = await cdp.send('Target.createTarget', { url: `chrome-extension://${extIdReal}/src/popup.html` });
  const popupSession = await cdp.attach(popup.targetId);
  await cdp.send('Page.enable', {}, popupSession);
  // The DOM exists as soon as the HTML is parsed, but the click handlers live
  // in a module script: clicking before the first render reaches the element is
  // a no-op, which is exactly the flake this wait removes.
  await waitFor(
    () => cdp!.eval(popupSession, `!!document.getElementById('pairBtn') && document.getElementById('state').textContent !== 'Connecting\u2026'`).catch(() => false),
    15000,
    'popup module script to render',
  );
  // 3a. Point the extension at the E2E API port through the popup UI (the
  //     shipped default is 7431, which is the user's real instance).
  await cdp.eval(
    popupSession,
    `(() => { const p = document.getElementById('port'); p.value = ${JSON.stringify(String(API_PORT))}; p.dispatchEvent(new Event('input')); document.getElementById('savePort').click(); return true; })()`,
  );
  await waitFor(
    () => cdp!.eval(popupSession, `document.getElementById('state').textContent.includes('Connected')`).catch(() => false),
    20000,
    'popup to report a connection',
  );
  check(
    'extension reached the API through the popup port field',
    (await cdp.eval(popupSession, `chrome.storage.local.get(['port']).then((s) => s.port)`)) === API_PORT,
    `port ${API_PORT}`,
  );
  // The pair box only appears once the background is actually connected.
  await waitFor(
    () => cdp!.eval(popupSession, `!document.getElementById('pairBox').hidden`).catch(() => false),
    10000,
    'pair box to appear',
  );
  await cdp.eval(
    popupSession,
    `(() => { document.getElementById('code').value = ${JSON.stringify(code)}; document.getElementById('pairBtn').click(); return true; })()`,
  );
  await waitFor(() => {
    try {
      return (readState().devices || []).length >= 1;
    } catch {
      return false;
    }
  }, 15000, 'pairing to complete');
  check('paired through the popup UI', (readState().devices || []).length === 1);
  await cdp.send('Target.closeTarget', { targetId: popup.targetId });

  // 4. Scripting layer: type + click in real Chrome, read the result back.
  const search = await scrape({ url: `http://localhost:${SITE_PORT}/search` });
  const searchJSON = JSON.stringify(search.json)?.slice(0, 400);
  check('scripting scrape returns 200', search.status === 200, searchJSON);
  check('recipe auto-matched by host', search.json?.provenance?.recipe === 'e2e-search@1', JSON.stringify(search.json?.provenance));
  check('result carries bridge provenance and untrusted mark',
    search.json?.provenance?.transport === 'bridge' && search.json?.provenance?.untrusted === true,
    JSON.stringify(search.json?.provenance));
  check('real Chrome ran type+click (post-action text present)',
    String(search.json?.markdown || '').includes('Echo: hello bridge'),
    String(search.json?.markdown || '').slice(0, 200));
  check('recipe schema extraction ran', search.json?.extracted?.heading === 'Fixture Search',
    JSON.stringify(search.json?.extracted));

  // 5. Turbo layer: trusted input through chrome.debugger + full-page shot.
  // 5. Turbo layer: trusted input through chrome.debugger + full-page shot.
  //    Fetched through the recipe's own host (turbo.test), which the browser is
  //    told to resolve locally, so the recipe's declared host is the one the
  //    request actually uses.
  const turbo = await scrape({ url: `http://turbo.test:${SITE_PORT}/search`, recipe: 'e2e-turbo', screenshot: true, screenshot_opts: { full_page: true } });
  check('turbo scrape returns 200', turbo.status === 200, JSON.stringify(turbo.json)?.slice(0, 400));
  check('turbo used the debugger layer', turbo.json?.provenance?.io === 'turbo', JSON.stringify(turbo.json?.provenance));
  check('turbo typed trusted input through the debugger (text present)', String(turbo.json?.markdown || '').includes('Echo: hello bridge'));
  check('turbo captured a full-page screenshot',
    typeof turbo.json?.screenshot?.blob === 'string' && turbo.json.screenshot.blob.length > 1000,
    JSON.stringify({ hasBlob: !!turbo.json?.screenshot?.blob, size: turbo.json?.screenshot?.size_bytes }));

  // 6. A link click runs the browser's own navigation. This settles two things
  //    at once: a synthetic click really does follow an <a href>, and following
  //    it INSIDE the origin the fetch was allowed to drive does not trip the
  //    cross-origin guard.
  const link = await scrape({ url: `http://link.test:${SITE_PORT}/search`, recipe: 'e2e-link' });
  check('click followed the link to the next page', link.status === 200 && String(link.json?.markdown || '').includes('Arrived by clicking the link'),
    JSON.stringify(link.json)?.slice(0, 300));
  check('same-origin navigation is allowed and recorded', String(link.json?.provenance?.recipe || '') === 'e2e-link@1',
    JSON.stringify(link.json?.provenance));

  // 7. The credential red line: a recipe that asks the bridge to type into a
  //    password field must be refused, and the field must stay empty.
  const login = await scrape({ url: `http://login.test:${SITE_PORT}/login`, recipe: 'e2e-login' });
  const failed = (login.json?.provenance?.actions_failed || []) as string[];
  check('typing into a password field is refused',
    failed.some((f) => f.toLowerCase().includes('credential')),
    JSON.stringify({ failed, provenance: login.json?.provenance }));

  // 8. The M1b evidence the plan asks for: the browser the bridge drives is not
  //    reporting itself as automated.
  check('the driven browser reports navigator.webdriver = false',
    String(login.json?.markdown || '').includes('webdriver: false'),
    String(login.json?.markdown || '').slice(0, 200));

  // 9. A challenge page stops the run: blocked, never content.
  const blocked = await scrape({ url: `http://challenge.test:${SITE_PORT}/challenge`, recipe: 'e2e-challenge' });
  check('challenge page is blocked, not content',
    blocked.status === 502 && blocked.json?.code === 'blocked',
    JSON.stringify(blocked.json)?.slice(0, 300));

  // 10. Audit log carries the runs with recipe, lane, origin.
  const audit = readFileSync(join(STATE_DIR, 'access.log.jsonl'), 'utf8')
    .trim().split('\n').map((l) => JSON.parse(l)).filter((e) => e.event === 'fetch');
  check('audit log recorded the bridged fetches',
    audit.length >= 3 && audit.some((e) => e.lane === 'borrow' && e.recipe === 'e2e-turbo'),
    JSON.stringify(audit.slice(-3)));
} finally {
  cdp?.close();
  chrome.kill();
  api.kill();
  site.stop();
}

assertAllPassed('E2E-B');
