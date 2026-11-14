// E2E-A: the bridge protocol against the real API, with a scripted "extension".
// This proves the server half end to end — pairing, fetch routing, recipe
// expectations, provenance, audit, and the offline-error path — without a
// browser. E2E-B is the same flow through the real extension in real Chrome.

import {
  resetRunDir, buildBinaries, startFixtureSite, startAPI, writePack, installPack, verifyPack,
  scrape, readState, STATE_DIR, BRIDGE_DIR, API_PORT, check, assertAllPassed,
} from './harness';
import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';

type Frame = Record<string, any>;

// hmacHex mirrors the extension's and the server's HMAC: same key, same exact
// message, so a proof computed here is the proof either side would compute.
async function hmacHex(key: string, message: string): Promise<string> {
  const k = await crypto.subtle.importKey(
    'raw', new TextEncoder().encode(key),
    { name: 'HMAC', hash: 'SHA-256' }, false, ['sign'],
  );
  const sig = await crypto.subtle.sign('HMAC', k, new TextEncoder().encode(message));
  return [...new Uint8Array(sig)].map((b) => b.toString(16).padStart(2, '0')).join('');
}

class FakeExtension {
  ws: WebSocket | null = null;
  token = '';
  serverKey = '';
  verified = false;
  paired = false;
  nonce = '';
  proofOk = false;
  fetched: Frame[] = [];
  handlers: Record<string, (msg: Frame) => void> = {};

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      this.ws = new WebSocket(`ws://127.0.0.1:${API_PORT}/bridge`);
      this.ws.onopen = () => resolve();
      this.ws.onerror = (e) => reject(new Error(`ws error ${e}`));
      this.ws.onmessage = (ev) => {
        const msg = JSON.parse(String(ev.data)) as Frame;
        this.fetched.push(msg);
        const h = this.handlers[msg.type];
        if (h) h(msg);
        // pings answered always
        if (msg.type === 'ping') this.send({ type: 'pong' });
      };
    });
  }

  send(msg: Frame) {
    this.ws!.send(JSON.stringify(msg));
  }

  /** Wait for a frame of a type we have not seen yet. */
  waitFrame(type: string, timeoutMs = 5000): Promise<Frame> {
    return new Promise((resolve, reject) => {
      const t = setTimeout(() => reject(new Error(`timeout waiting for ${type}; saw ${this.fetched.map((f) => f.type).join(',')}`)), timeoutMs);
      this.handlers[type] = (msg) => {
        clearTimeout(t);
        delete this.handlers[type];
        resolve(msg);
      };
    });
  }

  hello() {
    this.send({ type: 'hello', protocol: 1, browser: 'e2e-fake', version: '0.0.1' });
  }

  close() {
    try { this.ws?.close(); } catch { /* already closed */ }
  }
}

console.log('== E2E-A: bridge protocol against the real API ==');
resetRunDir();
const site = startFixtureSite();
await buildBinaries();
writePack();
const api = await startAPI();
await installPack();

try {
  // 1. Offline fixtures pass with no network.
  const verify = await verifyPack();
  check('recipes verify passes offline', verify.includes('exit=0') && verify.includes('all fixtures pass'), verify);

  // 2. The manifests ask for what the bridge needs and nothing else. The
  //    README claims the extension cannot read cookies or watch requests; this
  //    is the assertion that keeps that claim true.
  // The manifests are generated now (WXT), so assert what actually ships rather
  // than a hand-written source file: the build output is the artifact a user
  // loads.
  for (const target of ['chrome-mv3', 'firefox-mv3']) {
    if (!existsSync(join(BRIDGE_DIR, '.output', target, 'manifest.json'))) {
      const build = Bun.spawnSync(['bun', 'run', 'build'], {
        cwd: BRIDGE_DIR,
        env: { ...process.env, ...(target === 'firefox-mv3' ? { WXT_TARGET: 'firefox-mv3' } : {}) },
      });
      if (target === 'firefox-mv3') {
        Bun.spawnSync(['bun', 'run', 'build:firefox'], { cwd: BRIDGE_DIR });
      }
      if (build.exitCode !== 0) throw new Error(`could not build ${target}`);
    }
  }
  const chromeManifest = JSON.parse(
    readFileSync(join(BRIDGE_DIR, '.output', 'chrome-mv3', 'manifest.json'), 'utf8'),
  );
  const firefoxManifest = JSON.parse(
    readFileSync(join(BRIDGE_DIR, '.output', 'firefox-mv3', 'manifest.json'), 'utf8'),
  );
  const forbidden = ['cookies', 'webRequest', 'nativeMessaging', 'externally_connectable', 'history', 'bookmarks'];
  const perms = [...(chromeManifest.permissions || []), ...(firefoxManifest.permissions || []), ...(chromeManifest.optional_permissions || [])];
  check('no cookie, webRequest or nativeMessaging permissions',
    !perms.some((p) => forbidden.some((f) => String(p).toLowerCase().includes(f.toLowerCase()))),
    JSON.stringify(perms));
  check('the debugger permission is chrome-only (firefox has no such API)',
    perms.includes('debugger') && !(firefoxManifest.permissions || []).includes('debugger'),
    JSON.stringify({ chrome: chromeManifest.permissions, firefox: firefoxManifest.permissions }));

  // 2. Pairing: unknown connection is asked to pair; a wrong code fails; the
  //    right code returns a token; the token re-authenticates.
  const ext = new FakeExtension();
  await ext.connect();
  ext.hello();
  const challenge = await ext.waitFrame('challenge');
  ext.nonce = challenge.nonce;
  check('hello is answered with a challenge nonce and proof',
    typeof challenge.nonce === 'string' && challenge.nonce.length > 0 && typeof challenge.proof === 'string',
    JSON.stringify({ nonce: challenge.nonce, proofLen: String(challenge.proof).length }));

  // A code that is definitely not the real one (a hardcoded 000000 would
  // collide with the server's own code about once in a million runs).
  const wrongCode = ext.nonce ? String((Number(readState().code) + 1) % 1000000).padStart(6, '0') : '000000';
  ext.send({ type: 'pair', code: wrongCode, nonce: ext.nonce });
  const badPair = await ext.waitFrame('error');
  check('wrong pairing code is refused', badPair.code === 'pair-failed', JSON.stringify(badPair));

  const code = readState().code;
  ext.send({ type: 'pair', code, nonce: ext.nonce });
  const okPair = await ext.waitFrame('pair.ok');
  ext.token = okPair.token;
  ext.serverKey = okPair.server_key;
  check('correct code returns a device token', typeof ext.token === 'string' && ext.token.length === 64);

  // The extension verifies this proof with the code the user typed; if it did
  // not hold, a squatter on the port could fake a pairing.
  const pairProof = await hmacHex(code, `tomoshibi-pair|${ext.nonce}|${okPair.token}|${okPair.server_key}`);
  check('pair.ok proof verifies against the pairing code', pairProof === okPair.proof,
    JSON.stringify({ want: pairProof?.slice(0, 16), got: String(okPair.proof).slice(0, 16) }));
  check('pair.ok carries the server key the extension stores', typeof ext.serverKey === 'string' && ext.serverKey.length === 64);
  check('pairing code rotates after use', readState().code !== code);

  const ext2 = new FakeExtension();
  await ext2.connect();
  ext2.hello();
  const ch2 = await ext2.waitFrame('challenge');
  // The steady-state handshake: verify the server before releasing the token.
  const serverProof = await hmacHex(ext.serverKey, `tomoshibi-srv|${ch2.nonce}`);
  ext2.proofOk = serverProof === ch2.proof;
  check('challenge proof verifies with the stored server key', ext2.proofOk,
    JSON.stringify({ got: String(ch2.proof).slice(0, 16), want: String(serverProof).slice(0, 16) }));
  ext2.send({ type: 'auth', token: ext.token, nonce: ch2.nonce });
  const status = await ext2.waitFrame('status');
  check('stored token re-authenticates without a code', status.paired === true, JSON.stringify(status));
  // The newest connection owns the bridge (a browser restart must not leave a
  // zombie in charge), so every later step speaks through ext2, and the older
  // socket has to be closed by the server, not merely ignored.
  await Bun.sleep(300);
  check('newest connection replaces the older one', ext.ws?.readyState === WebSocket.CLOSED,
    `old socket readyState=${ext.ws?.readyState}`);

  // A fetch must not run before authentication, and the two failure shapes are
  // distinct: a bad token sends the client back to pairing, a replayed
  // handshake is refused outright. These run later, because any of them would
  // displace the connection the scrape checks below depend on.

  // 3. Fetch routing: the scripted extension answers every fetch by reading a
  //    local file; the server must route by recipe and build the result.
  const current = ext2;
  current.ws!.onmessage = (ev) => {
    const msg = JSON.parse(String(ev.data)) as Frame;
    current.fetched.push(msg);
    if (msg.type === 'ping') current.send({ type: 'pong' });
    if (msg.type === 'fetch') {
      const isChallenge = msg.url.includes('/challenge');
      const html = readFileSync(join(BRIDGE_DIR, 'fixtures', isChallenge ? 'challenge.html' : 'search-live.html'), 'utf8');
      const text = isChallenge ? 'Just a moment... Verifying you are human' : 'Fixture Search Echo: hello bridge';
      current.send({
        type: 'fetch.result', id: msg.id, status: 'ok',
        final_url: msg.url, title: isChallenge ? 'Just a moment...' : 'Fixture Search',
        text, html, io: msg.io || 'scripting',
        actions_run: (msg.actions || []).map((a: Frame) => ({ type: a.type, ok: true })),
      });
    }
  };

  const auto = await scrape({ url: `http://localhost:${7445}/search` });
  check('auto-match scrape returns 200', auto.status === 200, JSON.stringify(auto.json)?.slice(0, 300));
  check('auto-match used the search recipe', auto.json?.provenance?.recipe === 'e2e-search@1', JSON.stringify(auto.json?.provenance));
  check('result is marked bridge/borrow/untrusted',
    auto.json?.provenance?.transport === 'bridge' && auto.json?.provenance?.lane === 'borrow' && auto.json?.provenance?.untrusted === true,
    JSON.stringify(auto.json?.provenance));
  check('markdown carries the post-action text', String(auto.json?.markdown || '').includes('Echo: hello bridge'));
  check('template actions were forwarded and reported', (auto.json?.provenance?.actions_run ?? 0) >= 2);

  const explicit = await scrape({ url: `http://localhost:${7445}/search`, recipe: 'e2e-search' });
  check('explicit recipe scrape returns 200', explicit.status === 200);

  const missing = await scrape({ url: `http://localhost:${7445}/search`, recipe: 'nope' });
  check('unknown recipe fails loudly', missing.status === 500 && String(missing.json?.error || '').includes('not installed'), JSON.stringify(missing.json));

  // 4. A challenge page is a block, not content: the recipe's absent markers
  //    catch it even though the extension reported 200/ok.
  const blocked = await scrape({ url: `http://challenge.test/`, recipe: 'e2e-challenge' });
  check('challenge page returns 502 blocked', blocked.status === 502 && blocked.json?.code === 'blocked', JSON.stringify(blocked.json));

  // 5. Audit log records the fetches.
  const audit = readFileSync(join(STATE_DIR, 'access.log.jsonl'), 'utf8');
  const fetchLines = audit.trim().split('\n').map((l) => JSON.parse(l)).filter((e) => e.event === 'fetch');
  check('audit log has fetch lines with recipe and origin',
    fetchLines.length >= 3 && fetchLines.every((e) => e.origin && e.recipe),
    JSON.stringify(fetchLines.slice(-3)));

  // 6. Authentication failures, in both shapes. These replace the connection,
  //    so they come after everything that needs a paired browser.
  const ext3 = new FakeExtension();
  await ext3.connect();
  ext3.hello();
  const ch3 = await ext3.waitFrame('challenge');
  ext3.send({ type: 'auth', token: 'not-a-token', nonce: ch3.nonce });
  const stale = await ext3.waitFrame('pair.required');
  check('a stale token is sent back to pairing', stale.nonce === ch3.nonce, JSON.stringify(stale));
  ext3.close();

  const ext4 = new FakeExtension();
  await ext4.connect();
  ext4.hello();
  await ext4.waitFrame('challenge');
  ext4.send({ type: 'auth', token: 'not-a-token', nonce: 'deadbeef' });
  const replayed = await ext4.waitFrame('error');
  check('a replayed handshake is refused', replayed.code === 'refused', JSON.stringify(replayed));
  await Bun.sleep(200);
  check('the refused socket is closed', ext4.ws?.readyState === WebSocket.CLOSED, `readyState=${ext4.ws?.readyState}`);

  // 7. Offline bridge: no extension connected means a clear error, not a hang.
  current.close();
  await Bun.sleep(300);
  const offline = await scrape({ url: `http://localhost:${7445}/search` });
  check('offline bridge fails with a readable error',
    offline.status === 500 && String(offline.json?.error || '').includes('no browser connected'),
    JSON.stringify(offline.json));
} finally {
  api.kill();
  site.stop();
}

assertAllPassed('E2E-A');
