// Shared E2E harness: a fixture site, a real tomoshibi API on scratch ports,
// and a recipe pack built on the fly. Every temp path lives under a run dir so
// a failed run leaves evidence instead of mystery.

import { mkdirSync, writeFileSync, rmSync, readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { spawn, type Subprocess } from 'bun';

export const API_PORT = 7439;
export const SITE_PORT = 7445;
export const CDP_PORT = 9444;
export const API_KEY = 'e2e-key';

export const RUN_DIR = '/tmp/tomoshibi-e2e';
export const STATE_DIR = join(RUN_DIR, 'state');
export const PACK_DIR = join(RUN_DIR, 'pack');
export const CHROME_DIR = join(RUN_DIR, 'chrome');
// The extension repo is this checkout; deriving it from the file's own
// location keeps the tests runnable from anywhere.
export const BRIDGE_DIR = join(import.meta.dir, '..');
// The API package is a sibling now that the extension lives inside the
// monorepo: packages/bridge/test -> packages/api. Deriving it means the suite
// runs from any clone, in any directory.
export const REPO = join(import.meta.dir, '..', '..', 'api');

export const fixtureHTML = {
  home: `<!doctype html><html><head><title>Fixture Home</title></head>
<body><h1>Fixture Home</h1><p>A quiet page that needs no session.</p></body></html>`,
  search: `<!doctype html><html><head><title>Fixture Search</title></head>
<body>
  <h1>Fixture Search</h1>
  <input id="q" type="text" placeholder="query" />
  <button id="go">Search</button>
  <div id="results"></div>
  <p><a id="next" href="/results">Next page</a></p>
  <script>
    document.getElementById('go').addEventListener('click', () => {
      document.getElementById('results').textContent = 'Echo: ' + document.getElementById('q').value;
    });
  </script>
</body></html>`,
  // Reached only by clicking the link above: proves a synthetic click runs the
  // browser's own navigation, and that following it inside one origin is not
  // mistaken for the cross-origin drift the bridge refuses.
  results: `<!doctype html><html><head><title>Fixture Results</title></head>
<body><h1>Fixture Results</h1><p>Arrived by clicking the link.</p></body></html>`,
  // What the page looks like after the actions have run (used by the offline
  // fixture check; the live check is the contains marker).
  searchLive: `<!doctype html><html><head><title>Fixture Search</title></head>
<body><h1>Fixture Search</h1><input id="q" /><button id="go">Search</button>
<div id="results">Echo: hello bridge</div></body></html>`,
  // A page with a credential field, to prove the bridge refuses to type into
  // one (the plan's red line: no code path that types credentials).
  login: `<!doctype html><html><head><title>Fixture Login</title></head>
<body>
  <h1>Fixture Login</h1>
  <input id="user" type="text" name="username" />
  <input id="pass" type="password" name="password" />
  <button id="signin">Sign in</button>
  <p id="wd"></p>
  <script>document.getElementById('wd').textContent = 'webdriver: ' + navigator.webdriver;</script>
</body></html>`,
  challenge: `<!doctype html><html><head><title>Just a moment...</title></head>
<body><h1>Verifying you are human</h1><p>Enable JavaScript and cookies to continue</p></body></html>`,
};

export function resetRunDir() {
  rmSync(RUN_DIR, { recursive: true, force: true });
  mkdirSync(STATE_DIR, { recursive: true });
  mkdirSync(PACK_DIR, { recursive: true });
  mkdirSync(CHROME_DIR, { recursive: true });
}

// ---------------------------------------------------------------- fixture site

export function startFixtureSite() {
  const server = Bun.serve({
    port: SITE_PORT,
    hostname: '127.0.0.1',
    fetch(req) {
      const url = new URL(req.url);
      let body = fixtureHTML.home;
      if (url.pathname === '/search') body = fixtureHTML.search;
      if (url.pathname === '/results') body = fixtureHTML.results;
      if (url.pathname === '/login') body = fixtureHTML.login;
      if (url.pathname === '/challenge') body = fixtureHTML.challenge;
      return new Response(body, { headers: { 'content-type': 'text/html; charset=utf-8' } });
    },
  });
  return { stop: () => server.stop(true) };
}

// --------------------------------------------------------------------- API

export async function buildBinaries() {
  for (const [name, pkg] of [
    ['tomoshibi-api', './cmd/api'],
    ['tomoshibi-cli', './cmd/tomoshibi'],
  ] as const) {
    const proc = spawn(['go', 'build', '-o', join(RUN_DIR, name), pkg], { cwd: REPO, stdout: 'pipe', stderr: 'pipe' });
    const code = await proc.exited;
    if (code !== 0) throw new Error(`go build ${pkg} failed: ${await new Response(proc.stderr).text()}`);
  }
}

export async function startAPI(): Promise<Subprocess> {
  const proc = spawn([join(RUN_DIR, 'tomoshibi-api')], {
    cwd: REPO,
    env: {
      ...process.env,
      SERVER_PORT: String(API_PORT),
      SERVER_MODE: 'release',
      LOG_LEVEL: 'warn',
      APP_API_KEYS: API_KEY,
      DISABLE_WORKER: 'true',
      BRIDGE_STATE_DIR: STATE_DIR,
      REDIS_URL: '',
    },
    stdout: 'pipe',
    stderr: 'pipe',
  });
  // Surface startup failures instead of timing out blind.
  const reader = proc.stderr.getReader();
  const log: string[] = [];
  (async () => {
    const dec = new TextDecoder();
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      log.push(dec.decode(value));
    }
  })();
  try {
    await waitHttp(`http://127.0.0.1:${API_PORT}/health`, 15000);
  } catch (e) {
    throw new Error(`API did not start: ${e}\nstderr:\n${log.join('')}`);
  }
  return proc;
}

export async function waitHttp(url: string, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs;
  let lastErr: unknown;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (res.ok) return res;
      lastErr = new Error(`status ${res.status}`);
    } catch (e) {
      lastErr = e;
    }
    await Bun.sleep(200);
  }
  throw new Error(`waitHttp ${url}: ${lastErr}`);
}

export function readState(): { code: string; port?: string; devices?: unknown[]; code_at?: string } {
  return JSON.parse(readFileSync(join(STATE_DIR, 'bridge.json'), 'utf8'));
}

export async function apiPost(path: string, body: Record<string, unknown>) {
  const res = await fetch(`http://127.0.0.1:${API_PORT}${path}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', 'x-api-key': API_KEY },
    body: JSON.stringify(body),
  });
  let json: any = null;
  try {
    json = await res.json();
  } catch {
    /* error bodies may be empty */
  }
  return { status: res.status, json };
}

export async function scrape(body: Record<string, unknown>) {
  return apiPost('/v1/scrape', body);
}

// -------------------------------------------------------------- recipe pack

function sha256(text: string): string {
  return new Bun.CryptoHasher('sha256').update(text).digest('hex');
}

export function writePack() {
  mkdirSync(join(PACK_DIR, 'sites'), { recursive: true });
  mkdirSync(join(PACK_DIR, 'fixtures'), { recursive: true });

  const files: Record<string, string> = {
    'sites/e2e-search.yaml': `id: e2e-search
version: 1
match: { domains: ["localhost"] }
access: { lane: borrow, io: scripting, pace: { min_interval_ms: 0 }, allow_private: true }
request:
  actions:
    - { type: type, selector: "#q", text: "hello bridge" }
    - { type: click, selector: "#go" }
  wait_selector: "#results"
extract:
  schema:
    heading: { selector: "h1", attr: text }
expect:
  contains: ["Echo: hello bridge"]
fixtures:
  - { url: "http://localhost:${SITE_PORT}/search", file: "fixtures/search.html", expect: { heading: "Fixture Search" } }
`,
    'sites/e2e-turbo.yaml': `id: e2e-turbo
version: 1
match: { domains: ["turbo.test"] }
access: { lane: borrow, io: turbo, pace: { min_interval_ms: 0 }, allow_private: true }
request:
  actions:
    - { type: type, selector: "#q", text: "hello bridge" }
    - { type: click, selector: "#go" }
  wait_selector: "#results"
extract:
  schema:
    heading: { selector: "h1", attr: text }
expect:
  contains: ["Echo: hello bridge"]
fixtures:
  - { url: "http://turbo.test/search", file: "fixtures/search.html", expect: { heading: "Fixture Search" } }
`,
    'sites/e2e-link.yaml': `id: e2e-link
version: 1
match: { domains: ["link.test"] }
access: { lane: borrow, io: scripting, allow_private: true }
request:
  actions:
    - { type: click, selector: "#next" }
  wait_selector: "h1"
extract:
  schema:
    heading: { selector: "h1", attr: text }
expect:
  contains: ["Fixture Results"]
fixtures:
  - { url: "http://link.test/results", file: "fixtures/results.html", expect: { heading: "Fixture Results" } }
`,
    'sites/e2e-login.yaml': `id: e2e-login
version: 1
match: { domains: ["login.test"] }
access: { lane: borrow, io: scripting, allow_private: true }
request:
  actions:
    - { type: type, selector: "#user", text: "a-user" }
    - { type: type, selector: "#pass", text: "a-password" }
extract:
  schema:
    heading: { selector: "h1", attr: text }
fixtures:
  - { url: "http://login.test/login", file: "fixtures/login.html", expect: { heading: "Fixture Login" } }
`,
    'sites/e2e-challenge.yaml': `id: e2e-challenge
version: 1
match: { domains: ["challenge.test"] }
access: { lane: borrow, io: scripting, allow_private: true }
extract:
  schema:
    heading: { selector: "h1", attr: text }
expect:
  absent: ["Just a moment", "Verifying you are human"]
fixtures:
  - { url: "http://challenge.test/", file: "fixtures/search.html", expect: { heading: "Fixture Search" } }
`,
    'fixtures/search.html': fixtureHTML.searchLive,
    'fixtures/results.html': fixtureHTML.results,
    'fixtures/login.html': fixtureHTML.login,
  };

  for (const [rel, content] of Object.entries(files)) {
    writeFileSync(join(PACK_DIR, rel), content);
  }

  const manifest = [
    '# Generated by the E2E harness; every file is hash-listed.',
    'name: e2e-pack',
    'version: 1',
    'files:',
    ...Object.entries(files)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([rel, body]) => `  - { path: "${rel}", sha256: "${sha256(body)}" }`),
    '',
  ].join('\n');
  writeFileSync(join(PACK_DIR, 'manifest.yaml'), manifest);
  return PACK_DIR;
}

export async function installPack(): Promise<string> {
  const proc = spawn([join(RUN_DIR, 'tomoshibi-cli'), 'recipes', 'add', PACK_DIR], {
    env: { ...process.env, BRIDGE_STATE_DIR: STATE_DIR },
    stdout: 'pipe',
    stderr: 'pipe',
  });
  const out = await new Response(proc.stdout).text();
  const code = await proc.exited;
  if (code !== 0) throw new Error(`recipes add failed: ${out} ${await new Response(proc.stderr).text()}`);
  return out;
}

export async function verifyPack(): Promise<string> {
  const proc = spawn([join(RUN_DIR, 'tomoshibi-cli'), 'recipes', 'verify'], {
    env: { ...process.env, BRIDGE_STATE_DIR: STATE_DIR },
    stdout: 'pipe',
    stderr: 'pipe',
  });
  const out = await new Response(proc.stdout).text();
  const code = await proc.exited;
  const err = await new Response(proc.stderr).text();
  return `exit=${code}\n${out}${err}`;
}

// ---------------------------------------------------------------- assertions

let failures = 0;

export function check(name: string, ok: boolean, detail = '') {
  if (ok) {
    console.log(`  ok   ${name}`);
  } else {
    failures++;
    console.log(`  FAIL ${name}${detail ? `\n       ${detail}` : ''}`);
  }
}

export function assertAllPassed(label: string) {
  if (failures > 0) {
    console.log(`\n${label}: ${failures} check(s) FAILED`);
    process.exit(1);
  }
  console.log(`\n${label}: all checks passed`);
}

export function cleanup() {
  if (existsSync(CHROME_DIR)) {
    // Chrome is killed by the caller; this only clears scratch data.
  }
}
