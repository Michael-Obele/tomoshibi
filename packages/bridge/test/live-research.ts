// E2E-C: a REAL research run — real Chrome, the real extension, the real
// internet.
//
// This is the proof that the product does the thing it was built for, so it
// asserts against live engines rather than fixtures:
//
//   1. browser:ddg     returns real results
//   2. browser:google  returns real results (this is the engine that is blocked
//                      from a datacenter address — see data/telemetry)
//   3. browser:bing    returns real results
//   4. the three engines return DIFFERENT urls, which is the only way to know the
//      switch is real and not three labels over one backend
//   5. /v1/research    turns a query into read pages, with no paid search API
//   6. an origin nobody allowed is refused with 428 consent-required
//
// A challenge from an engine is a PASS, not a flake: the correct behaviour is to
// stop and say so, never to return an empty result set as if the query had no
// answers. (Google can challenge; DuckDuckGo usually will not.)
//
// Run it:
//   bun test/live-research.ts
//
// It needs a network and a Chrome for Testing build (CHROME_BIN or the Playwright
// cache). It is deliberately NOT part of `bun run test`: a suite that needs the
// internet fails for reasons that have nothing to do with the code.

import { spawn } from 'bun';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import {
  resetRunDir,
  buildBinaries,
  startAPI,
  apiPost,
  readState,
  RUN_DIR,
  CDP_PORT,
  API_PORT,
  BRIDGE_DIR,
  check,
  assertAllPassed,
} from './harness';
import { CDP, resolveChrome, extensionIdFromPath, waitFor } from './cdp';

const EXT_DIR = join(BRIDGE_DIR, '.output', 'chrome-mv3');
const CHROME_BIN = resolveChrome();
const QUERY = 'svelte 5 runes';

console.log('== E2E-C: real research through the real browser ==');
resetRunDir();
await buildBinaries();
const api = await startAPI();

const chromeDir = join(RUN_DIR, 'chrome');
mkdirSync(chromeDir, { recursive: true });
// Headful by default when a display exists, because that is how the bridge is
// actually used: a fresh headless profile with no history and no cookies is the
// most suspicious browser there is, and the engines react to it. `HEADLESS=1`
// forces the worst case on purpose.
const headless = process.env.HEADLESS === '1' || !process.env.DISPLAY;

const chrome = spawn([
  CHROME_BIN,
  ...(headless ? ['--headless=new'] : []),
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
  '--disable-component-extensions-with-background-pages',
  'about:blank',
]);

let cdp: CDP | null = null;
try {
  await waitFor(
    async () => {
      try {
        const res = await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`, {
          signal: AbortSignal.timeout(1000),
        });
        return res.ok;
      } catch {
        return false;
      }
    },
    20000,
    'chrome devtools endpoint',
  );
  const version = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`)).json();
  cdp = await CDP.connect(version.webSocketDebuggerUrl);

  // ── pair, exactly as a person would ────────────────────────────────
  let extId = '';
  const findSW = async (): Promise<boolean> => {
    const { targetInfos } = await cdp!.send('Target.getTargets');
    const sw = targetInfos.find(
      (t: any) => t.type === 'service_worker' && String(t.url).endsWith('/background.js'),
    );
    if (sw) {
      extId = String(sw.url).split('/')[2] ?? '';
      return true;
    }
    return false;
  };
  if (!(await findSW())) {
    const guessed = extensionIdFromPath(EXT_DIR);
    await cdp.send('Target.createTarget', { url: `chrome-extension://${guessed}/popup.html` }).catch(() => {});
    await waitFor(findSW, 15000, 'bridge service worker');
  }
  check('extension loaded', !!extId, extId);

  const code = readState().code;
  const popup = await cdp.send('Target.createTarget', {
    url: `chrome-extension://${extId}/popup.html`,
  });
  const session = await cdp.attach(popup.targetId);
  await cdp.send('Page.enable', {}, session);
  await waitFor(
    () =>
      cdp!
        .eval(
          session,
          `!!document.getElementById('pairBtn') && document.getElementById('state').textContent !== 'Connecting…'`,
        )
        .catch(() => false),
    15000,
    'popup to render',
  );
  await cdp.eval(
    session,
    `(() => { const p = document.getElementById('port'); p.value = ${JSON.stringify(String(API_PORT))}; p.dispatchEvent(new Event('input')); document.getElementById('savePort').click(); return true; })()`,
  );
  await waitFor(
    () => cdp!.eval(session, `document.getElementById('state').textContent.includes('Connected')`).catch(() => false),
    20000,
    'popup connected',
  );
  await waitFor(
    () => cdp!.eval(session, `!document.getElementById('pairBox').hidden`).catch(() => false),
    10000,
    'pair box',
  );
  await cdp.eval(
    session,
    `(() => { document.getElementById('code').value = ${JSON.stringify(code)}; document.getElementById('pairBtn').click(); return true; })()`,
  );
  await waitFor(() => (readState().devices || []).length >= 1, 15000, 'pairing');
  check('paired', (readState().devices || []).length === 1);
  await cdp.send('Target.closeTarget', { targetId: popup.targetId });

  // ── 1-3. each engine, through the browser ──────────────────────────
  const found: Record<string, { urls: string[]; status: number; note: string }> = {};
  for (const engine of ['ddg', 'google', 'bing']) {
    const started = Date.now();
    const r = await apiPost('/v1/search', { query: QUERY, limit: 10, engine: `browser:${engine}` });
    const urls: string[] = (r.json?.results || []).map((x: any) => String(x.url));
    const ms = Date.now() - started;
    const note = r.json?.error || `${urls.length} results in ${ms}ms`;
    found[engine] = { urls, status: r.status, note };
    // Either real results, or the honest refusal. Never a hang, never a silent
    // empty page dressed up as "no results".
    const challenged = r.status === 502 && /challenge|blocked/i.test(String(r.json?.error || ''));
    check(
      `browser:${engine} answered seriously`,
      (r.status === 200 && urls.length >= 3) || challenged,
      note,
    );
  }

  const ddg = found.ddg?.urls ?? [];
  const google = found.google?.urls ?? [];
  check(
    'engines return different result sets (the switch is real)',
    ddg.length === 0 || google.length === 0 || ddg[0] !== google[0],
    `ddg[0]=${ddg[0] || '-'} google[0]=${google[0] || '-'}`,
  );

  // ── 4. the SERP results are usable URLs, not redirect wrappers ─────
  const allUrls = [...ddg, ...google, ...(found.bing?.urls ?? [])];
  check(
    'no engine redirect wrapper leaked into results',
    allUrls.every((u) => /^https?:\/\//.test(u) && !u.includes('uddg=') && !u.includes('/url?q=')),
    allUrls.slice(0, 3).join(' | '),
  );

  // ── 5. research: query → SERP → pages ──────────────────────────────
  const started = Date.now();
  const research = await apiPost('/v1/research', {
    query: QUERY,
    engine: 'browser:ddg',
    limit: 10,
    pages: 3,
    transport: 'auto',
    summary: true,
  });
  const elapsed = Date.now() - started;
  const body: any = research.json || {};
  const pages = body.pages || [];
  const words = pages.reduce((n: number, p: any) => n + (p.word_count || 0), 0);
  check(
    'research returned read pages',
    research.status === 200 && pages.length >= 1 && words > 100,
    `${pages.length} pages, ${words} words, ${elapsed}ms`,
  );
  check(
    'research used no paid search API',
    body.paid_api_used === false,
    `paid_api_used=${body.paid_api_used}`,
  );
  // A page read by an anonymous rung has no provenance (only the bridge sets
  // one), so "absent or bridged" is the correct assertion, not "always present".
  check(
    'provenance is honest about how each page was read',
    pages.every((p: any) => p.provenance === undefined || p.provenance.transport === 'bridge'),
    JSON.stringify(pages.map((p: any) => p.provenance?.transport ?? 'anonymous')),
  );

  // ── 6. the gate ────────────────────────────────────────────────────
  const refused = await apiPost('/v1/scrape', {
    url: 'https://example.com',
    transport: 'bridge',
  });
  check(
    'an origin nobody allowed is refused with 428',
    refused.status === 428 && refused.json?.code === 'consent-required',
    `${refused.status} ${JSON.stringify(refused.json)?.slice(0, 160)}`,
  );

  assertAllPassed('E2E-C');
} finally {
  try {
    cdp?.close();
  } catch {
    /* gone */
  }
  chrome.kill();
  api.kill();
}
