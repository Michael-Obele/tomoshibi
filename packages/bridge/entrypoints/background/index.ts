// Tomoshibi Bridge — service worker.
//
// One job: keep a loopback WebSocket to the local Tomoshibi API, and when it
// asks, read or drive a page inside this browser (the one you are signed into)
// and return what the page shows. Two IO layers:
//   scripting (default) — DOM events through the extension scripting API.
//                         No debugger, no infobar, works on Firefox.
//   turbo               — chrome.debugger CDP: trusted input, full-page and
//                         accessibility-tree reads. Chrome only; shows the
//                         debugging infobar.
//
// It never types credentials, never exports cookies, never reads tabs it was
// not asked about. Everything it does lands in Tomoshibi's access log.

import { defineBackground } from 'wxt/utils/define-background';
import { browser } from 'wxt/browser';
import {
  BRIDGE_PATH,
  BUILTIN_MARKERS,
  DEFAULT_PORT,
  PROTOCOL,
  type Action,
  type BridgeState,
  type FetchRequest,
  type FetchResult,
  type IO,
} from '../../utils/protocol';
import {
  detectMarkers,
  isCredentialField,
  originDrift,
  stopReason,
  type FieldAttrs,
} from '../../utils/guard';
import {
  clickBySelector,
  pageMotion,
  pageTreeDom,
  pressKey,
  readFieldAttrs,
  readPage,
  scrollAndCollect,
  settlePage,
  typeIntoSelector,
} from '../../utils/page';

// Named built-ins this build understands. Advertised in `hello` so a server can
// refuse to ask for something the extension does not have, instead of treating
// the omission as a successful empty result.
const FEATURES = ['settle', 'scroll_collect', 'page_tree', 'motion'];

// Page read caps. The server's frame limit has to hold HTML, text, tree and the
// image at once, so one place decides how much of a page comes back.
const LIMIT_HTML = 8_000_000;
const LIMIT_TEXT = 2_000_000;
const LIMIT_SHOT_B64 = 8_000_000;

const HEARTBEAT = 'tomoshibi-bridge-heartbeat';

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectDelay = 2000;
let ownToken = ''; // the token this worker wrote itself; ignore its storage echo
// lastPairCode holds the typed code only long enough to verify the server's
// reply; lastNonce is the challenge nonce the pair frame must echo.
let lastPairCode = '';
let lastNonce = '';
let paused = false;
let paired = false;
// verified means the process answering on the port proved it holds the server
// key from pairing. Nothing is sent to it and nothing runs for it until then,
// so a squatter on the port gets neither the token nor a browser it can drive.
let verified = false;
let lastError = '';
let connected = false;
let allowed: string[] = [];
let lastRun: BridgeState['lastRun'] = null;

const tabsByOrigin = new Map<string, number>(); // origin -> tabId (one pinned tab per origin)
const cancelled = new Set<string>(); // fetch ids the server cancelled
let queue: Promise<void> = Promise.resolve(); // serializes runs (v1: one at a time)

export default defineBackground(() => {
  // ------------------------------------------------------------- connection

  function badge(text: string, color?: string) {
    try {
      browser.action.setBadgeText({ text });
      if (color) browser.action.setBadgeBackgroundColor({ color });
    } catch {
      /* badge is cosmetic; never let it break the bridge */
    }
  }

  function send(msg: Record<string, unknown>): boolean {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(msg));
      return true;
    }
    return false;
  }

  interface Settings {
    token: string;
    serverKey: string;
    port: number;
    paused: boolean;
  }

  async function getSettings(): Promise<Settings> {
    const stored = (await browser.storage.local.get(['token', 'serverKey', 'port', 'paused'])) as
      | { token?: string; serverKey?: string; port?: number; paused?: boolean }
      | undefined;
    return {
      token: stored?.token || '',
      serverKey: stored?.serverKey || '',
      port: Number(stored?.port) || DEFAULT_PORT,
      paused: !!stored?.paused,
    };
  }

  async function connect() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
    const { port } = await getSettings();
    let socket: WebSocket;
    try {
      socket = new WebSocket(`ws://127.0.0.1:${port}${BRIDGE_PATH}`);
    } catch {
      scheduleReconnect();
      return;
    }
    ws = socket;
    // Every handler ignores events from a socket that is no longer current: a
    // late `close` from a replaced socket must not flip the badge off or queue
    // another reconnect.
    socket.onopen = () => {
      if (socket !== ws) return;
      connected = true;
      reconnectDelay = 2000;
      lastError = '';
      // The token is deliberately NOT in the hello: anything can bind a port
      // while Tomoshibi is down, and a squatter must not be handed the
      // credential just for answering. It goes out only after the server has
      // proved it knows the key saved at pairing (see onChallenge).
      verified = false;
      send({
        type: 'hello',
        protocol: PROTOCOL,
        browser: navigator.userAgent.includes('Firefox') ? 'firefox' : 'chrome',
        version: browser.runtime.getManifest().version,
        features: FEATURES,
      });
      broadcast();
    };

    socket.onmessage = (ev) => {
      if (socket !== ws) return;
      let msg: Record<string, unknown>;
      try {
        msg = JSON.parse(String(ev.data));
      } catch {
        return;
      }
      handleFrame(msg).catch((e) => console.warn('[bridge] frame failed', e));
    };

    socket.onclose = () => {
      if (socket !== ws) return;
      connected = false;
      paired = false;
      verified = false;
      badge('off', '#b91c1c');
      broadcast();
      scheduleReconnect();
    };

    socket.onerror = () => {
      try {
        socket.close();
      } catch {
        /* onclose handles it */
      }
    };
  }

  function scheduleReconnect() {
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(connect, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, 30000);
  }

  // Drop the current socket and come back promptly with fresh settings.
  // Deliberate changes (new port, revoked token) must not wait out a backoff
  // that has already grown to 30s, and `close()` on an already-closed socket
  // fires no event at all, so the reconnect is scheduled here rather than
  // hoped for.
  function restartSocket(delay = 150) {
    reconnectDelay = 2000;
    const old = ws;
    ws = null;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (old) {
      try {
        old.close();
      } catch {
        /* already gone */
      }
    }
    reconnectTimer = setTimeout(connect, delay);
  }

  // -------------------------------------------------------------- handshake

  // hmacHex is the extension's half of the pairing handshake: HMAC-SHA256 in
  // the exact shape the server uses, so both sides can check each other.
  async function hmacHex(key: string, message: string): Promise<string> {
    const enc = new TextEncoder();
    const k = await crypto.subtle.importKey('raw', enc.encode(key), { name: 'HMAC', hash: 'SHA-256' }, false, [
      'sign',
    ]);
    const sig = await crypto.subtle.sign('HMAC', k, enc.encode(message));
    return [...new Uint8Array(sig)].map((b) => b.toString(16).padStart(2, '0')).join('');
  }

  // challenge answers hello. If we hold a server key we verify the proof before
  // sending the token; if we do not (never paired), there is nothing to release
  // and nothing to verify, so we wait to be told whether pairing is needed.
  async function onChallenge(raw: Record<string, unknown>) {
    const nonce = String(raw.nonce || '');
    const proof = String(raw.proof || '');
    lastNonce = nonce;
    const { token, serverKey } = await getSettings();
    if (!serverKey) return; // unpaired: the token slot is empty anyway
    const want = await hmacHex(serverKey, `tomoshibi-srv|${nonce}`);
    if (want !== proof) {
      // Something else is answering the port, or the pairing was revoked and
      // the server minted a new key. Either way: no token, no fetches. The
      // socket stays open so the user can pair again from the popup — pairing
      // is verified against the code they type, which is the only proof
      // available before any key is shared.
      verified = false;
      paired = false;
      lastError = `unverified server on port ${(await getSettings()).port}: it does not hold our pairing key. If Tomoshibi was reinstalled or its state deleted, pair again.`;
      badge('err', '#b91c1c');
      broadcast();
      return;
    }
    verified = true;
    send({ type: 'auth', token, nonce });
  }

  async function handleFrame(msg: Record<string, any>) {
    switch (msg.type) {
      case 'challenge':
        await onChallenge(msg);
        break;
      case 'pair.required':
        paired = false;
        badge('pair', '#b45309');
        broadcast();
        break;
      case 'pair.ok': {
        // Verify the server knew the code the user typed. Without this,
        // whatever answered the port could hand us a token and claim a
        // successful pairing.
        const code = lastPairCode;
        lastPairCode = '';
        const want = await hmacHex(code, `tomoshibi-pair|${msg.nonce}|${msg.token}|${msg.server_key}`);
        if (!code || want !== msg.proof) {
          verified = false;
          paired = false;
          ownToken = '';
          lastError = 'refused to pair: the process answering the port does not know the pairing code';
          badge('err', '#b91c1c');
          broadcast();
          if (ws)
            try {
              ws.close();
            } catch {
              /* reconnect */
            }
          break;
        }
        // Remember what we wrote so the storage echo below is not mistaken for
        // a settings change (which would close the socket we just paired on).
        ownToken = msg.token;
        await browser.storage.local.set({ token: msg.token, serverKey: msg.server_key });
        verified = true;
        paired = true;
        lastError = '';
        badge('on', '#15803d');
        broadcast();
        break;
      }
      case 'status':
        paired = !!msg.paired && verified;
        if (Array.isArray(msg.allowed)) allowed = msg.allowed.map(String);
        badge(paired ? 'on' : 'pair', paired ? '#15803d' : '#b45309');
        broadcast();
        break;
      case 'ping':
        send({ type: 'pong' });
        break;
      case 'error':
        lastError = msg.message || msg.code || 'bridge error';
        badge('err', '#b91c1c');
        broadcast();
        break;
      case 'fetch':
        // Only an authenticated, paired server gets to drive this browser.
        if (!verified || !paired) {
          send({
            type: 'fetch.result',
            id: msg.id,
            status: 'error',
            message: 'extension is not paired with this server',
          });
          break;
        }
        queueRun(msg as FetchRequest);
        break;
      case 'cancel':
        cancelled.add(String(msg.id));
        break;
      default:
        break;
    }
  }

  function queueRun(req: FetchRequest) {
    queue = queue
      .then(() => runFetch(req))
      .catch((e) => {
        send({
          type: 'fetch.result',
          id: req.id,
          status: 'error',
          message: String((e && e.message) || e),
        });
      });
  }

  // -------------------------------------------------------------- popup API

  browser.runtime.onMessage.addListener((msg: any, _sender, reply) => {
    (async () => {
      switch (msg.kind) {
        case 'status': {
          const { port } = await getSettings();
          reply({ connected, paired, paused, lastError, port, allowed, lastRun } satisfies BridgeState);
          break;
        }
        case 'pair': {
          lastPairCode = String(msg.code || '').trim(); // kept only until pair.ok verifies it
          const sent = send({ type: 'pair', code: lastPairCode, nonce: lastNonce });
          if (!sent) lastError = 'not connected to Tomoshibi; the code was not sent';
          reply({ sent });
          break;
        }
        case 'forget':
          // The documented way to revoke is deleting the server's state file,
          // which mints a new server key. Without this the extension would hold
          // a key the server no longer has and never authenticate again.
          ownToken = '';
          verified = false;
          paired = false;
          allowed = [];
          lastError = '';
          await browser.storage.local.remove(['token', 'serverKey']);
          restartSocket();
          reply({ forgotten: true });
          break;
        case 'pause':
          paused = !!msg.on;
          await browser.storage.local.set({ paused });
          broadcast();
          reply({ paused });
          break;
        case 'port':
          await browser.storage.local.set({ port: Number(msg.port) || DEFAULT_PORT });
          restartSocket();
          reply({ saved: true });
          break;
        default:
          reply({ error: 'unknown message kind' });
      }
    })();
    return true; // async reply
  });

  function broadcast() {
    browser.runtime
      .sendMessage({ kind: 'state', connected, paired, paused, lastError, allowed, lastRun })
      .catch(() => {
        /* no popup open */
      });
  }

  // -------------------------------------------------------------- fetch run

  async function runFetch(req: FetchRequest) {
    const started = Date.now();
    const reply = (extra: Partial<FetchResult>) =>
      send({ type: 'fetch.result', id: req.id, ms: Date.now() - started, ...extra });
    const isCancelled = () => cancelled.has(req.id);

    try {
      if (paused) {
        reply({ status: 'paused', message: 'bridge is paused' });
        return;
      }
      if (!verified || !paired) {
        reply({ status: 'error', message: 'extension is not paired with this server' });
        return;
      }
      // A queued run may have been cancelled while it waited its turn; do not
      // navigate the user's tab for work nobody is waiting for.
      if (isCancelled()) {
        reply({ status: 'error', message: 'cancelled' });
        return;
      }
      const io: IO = req.io === 'turbo' ? 'turbo' : 'scripting';
      const origin = new URL(req.url).origin;
      const tabId = await tabFor(req.url, req.timeout_ms);
      if (isCancelled()) {
        reply({ status: 'error', message: 'cancelled' });
        return;
      }

      const out =
        io === 'turbo'
          ? await runTurbo(req, tabId, isCancelled, origin)
          : await runScripting(req, tabId, isCancelled, origin);

      // The page may have navigated itself (a redirect, a meta refresh, a click
      // that leads somewhere else). Driving on into a different origin, or
      // reading it back as if it were the requested page, is exactly the
      // cross-origin step this bridge must not take quietly. The per-action
      // checks inside the layers catch it during the run; this is the last word.
      const drift = originDrift(origin, out.final_url);
      if (drift) {
        reply({ status: 'error', message: drift });
        return;
      }
      lastRun = { origin, ms: Date.now() - started, actions: out.actions_run?.length || 0, at: Date.now() };
      reply(out);
      broadcast();
    } catch (e) {
      const message = String((e as { message?: unknown })?.message || e);
      // "paused" is a distinct outcome, not an error: the server reports it as
      // the paused code rather than a failure to fetch.
      reply({ status: message === 'paused' ? 'paused' : 'error', message });
    } finally {
      cancelled.delete(req.id);
    }
  }

  // assertSameOrigin throws when the tab has left the origin the fetch was
  // allowed to drive. Called between actions, not only at the end: by the time
  // the read-back happens, the typing has already happened.
  async function assertSameOrigin(tabId: number, wantOrigin: string) {
    let tab;
    try {
      tab = await browser.tabs.get(tabId);
    } catch {
      throw new Error('tab closed');
    }
    const drift = originDrift(wantOrigin, tab && tab.url);
    if (drift) throw new Error(drift);
  }

  // One pinned tab per origin, reused across fetches, left open on a handoff so
  // the human can clear whatever it is and retry.
  async function tabFor(rawUrl: string, timeoutMs?: number): Promise<number> {
    const origin = new URL(rawUrl).origin;
    const existingId = tabsByOrigin.get(origin);
    if (existingId != null) {
      try {
        await browser.tabs.get(existingId);
        await browser.tabs.update(existingId, { url: rawUrl, active: true });
        await waitTabComplete(existingId, timeoutMs);
        return existingId;
      } catch {
        // The pinned tab is unusable (closed, or it never finished loading).
        // Drop it rather than leaving it in the window for the next fetch to
        // duplicate.
        tabsByOrigin.delete(origin);
        try {
          await browser.tabs.remove(existingId);
        } catch {
          /* already gone */
        }
      }
    }
    const tab = await browser.tabs.create({ url: rawUrl, active: true });
    if (tab.id == null) throw new Error('tab could not be created');
    tabsByOrigin.set(origin, tab.id);
    await waitTabComplete(tab.id, timeoutMs);
    return tab.id;
  }

  async function waitTabComplete(tabId: number, timeoutMs = 45000) {
    const deadline = Date.now() + timeoutMs;
    // Give the navigation a moment to leave the previous 'complete' state.
    await sleep(150);
    while (Date.now() < deadline) {
      let tab;
      try {
        tab = await browser.tabs.get(tabId);
      } catch {
        throw new Error('tab closed');
      }
      if (!tab) throw new Error('tab closed');
      if (tab.status === 'complete') return;
      await sleep(200);
    }
    throw new Error('tab did not finish loading in time');
  }

  // --------------------------------------------------------- shared tail

  // Everything both layers do after the actions have run: settle, collect,
  // read, marker detection, tree and motion. One implementation so the two
  // layers cannot drift apart on the part the server actually consumes.
  async function readViaInject(
    tabId: number,
    req: FetchRequest,
    isCancelled: () => boolean,
    origin: string,
    actionsRun: FetchResult['actions_run'],
  ): Promise<FetchResult> {
    const out: FetchResult = { status: 'ok', io: 'scripting', actions_run: actionsRun, settled_ms: -1 };

    // scroll_collect runs before settle and before the read: the whole point is
    // that the content does not exist until you have scrolled to it.
    if (req.scroll_collect) {
      const collected = (await inject(tabId, scrollAndCollect as (...a: any[]) => any, [
        req.scroll_collect,
      ])) as FetchResult['collected'];
      out.collected = collected;
      if (stopReason(isCancelled(), paused)) {
        out.status = stopReason(isCancelled(), paused) as FetchResult['status'];
        return out;
      }
      await assertSameOrigin(tabId, origin);
    }

    if (req.settle) {
      const settled = (await inject(tabId, settlePage as (...a: any[]) => any, [req.settle])) as {
        settled: boolean;
        ms: number;
        reason: string;
      };
      out.settled_ms = settled?.ms ?? -1;
    } else {
      await sleep(req.settle_ms || 300);
    }
    await assertSameOrigin(tabId, origin);

    const page = (await inject(tabId, readPage as (...a: any[]) => any, [LIMIT_HTML, LIMIT_TEXT])) as {
      url: string;
      title: string;
      text: string;
      html: string;
      truncated: boolean;
      status: number;
    };
    out.final_url = page.url;
    out.title = page.title;
    out.text = page.text;
    out.html = page.html;
    out.html_truncated = page.truncated;
    if (page.status) out.http_status = page.status;

    const markers = detectMarkers(page, BUILTIN_MARKERS);
    if (markers.length) {
      out.status = 'awaiting-human';
      out.markers = markers;
    }

    if (req.page_tree === 'dom' || req.page_tree === 'both') {
      out.tree = (await inject(tabId, pageTreeDom as (...a: any[]) => any, [
        req.page_tree_options || {},
      ])) as FetchResult['tree'];
    }
    if (req.motion) {
      out.motion = (await inject(tabId, pageMotion as (...a: any[]) => any, [])) as FetchResult['motion'];
    }

    if (req.screenshot) {
      await scriptingScreenshot(tabId, out, actionsRun!);
    }
    return out;
  }

  function sleep(ms: number) {
    return new Promise((r) => setTimeout(r, ms));
  }

  async function inject(tabId: number, func: (...args: any[]) => any, args: any[] = []): Promise<unknown> {
    const results = await browser.scripting.executeScript({ target: { tabId }, func, args });
    const first = results && results[0];
    if (!first) throw new Error('script injection returned nothing');
    // Chrome rejects on a throwing injection; Firefox reports it on the result.
    const injected = first as { result?: unknown; error?: unknown };
    if (injected.error) {
      const err = injected.error as { message?: unknown } | string;
      throw new Error(String(typeof err === 'string' ? err : err?.message || err));
    }
    return injected.result;
  }

  // ---------------------------------------------------------- scripting layer

  async function runScripting(
    req: FetchRequest,
    tabId: number,
    isCancelled: () => boolean,
    origin: string,
  ): Promise<FetchResult> {
    const actionsRun: NonNullable<FetchResult['actions_run']> = [];

    if (req.wait_selector) {
      // The request-level wait is the assertion that the page it wants has
      // loaded. It is fatal on both layers: returning a page that never
      // rendered is the silent failure the recipes exist to prevent. Inside
      // `actions`, a wait stays advisory (a missed click should not lose a page).
      await scriptingWaitSelector(tabId, req.wait_selector, req.timeout_ms);
    }
    for (const action of req.actions || []) {
      const stop = stopReason(isCancelled(), paused);
      if (stop) return { status: stop, io: 'scripting', actions_run: actionsRun, message: stop };
      // Between actions, not just at the end: by the time the read-back happens
      // the typing has already happened.
      await assertSameOrigin(tabId, origin);
      actionsRun.push(await runScriptingAction(tabId, action, req.timeout_ms, origin));
    }

    return readViaInject(tabId, req, isCancelled, origin, actionsRun);
  }

  async function runScriptingAction(
    tabId: number,
    action: Action,
    timeoutMs: number | undefined,
    origin: string,
  ) {
    const done = (ok: boolean, error?: unknown) => ({
      type: action.type,
      ok,
      error: error ? String((error as any)?.message || error) : undefined,
    });
    try {
      switch (action.type) {
        case 'wait_ms':
          await sleep(action.ms || 1000);
          return done(true);
        case 'wait_selector': {
          const [ok, error] = await toTuple(scriptingWaitSelector(tabId, action.selector!, timeoutMs));
          return done(ok, error);
        }
        case 'wait_for_function':
          return done(false, 'wait_for_function requires io: turbo');
        case 'evaluate':
          return done(false, 'evaluate requires io: turbo');
        case 'scroll_to_bottom':
          await inject(tabId, () => window.scrollTo(0, document.body.scrollHeight));
          return done(true);
        case 'scroll_by':
          await inject(tabId, (px: number) => window.scrollBy(0, px), [action.px || 800]);
          return done(true);
        case 'scroll_down':
          await inject(tabId, (px: number) => window.scrollBy(0, px || window.innerHeight), [action.px || 0]);
          return done(true);
        case 'click':
          await inject(tabId, clickBySelector as (...a: any[]) => any, [action.selector]);
          return done(true);
        case 'type': {
          // The bridge drives pages, never accounts: a password, one-time code
          // or card field is refused outright rather than trusted to a recipe.
          const field = (await inject(tabId, readFieldAttrs as (...a: any[]) => any, [
            action.selector,
          ])) as FieldAttrs & { missing?: boolean };
          if (field && !field.missing && isCredentialField(field)) {
            return done(
              false,
              `refusing to type into a credential field (${action.selector}): the bridge never enters passwords or codes`,
            );
          }
          await inject(tabId, typeIntoSelector as (...a: any[]) => any, [action.selector, action.text || '']);
          return done(true);
        }
        case 'press':
          await inject(tabId, pressKey as (...a: any[]) => any, [action.key || 'Enter']);
          return done(true);
        case 'navigate': {
          // A navigate that leaves the origin would be driving another site with
          // the user's session; the request declares the origin it may touch.
          const drift = originDrift(origin, action.url);
          if (drift) return done(false, drift);
          await browser.tabs.update(tabId, { url: action.url });
          await waitTabComplete(tabId, timeoutMs);
          return done(true);
        }
        default:
          return done(false, `unknown action ${action.type}`);
      }
    } catch (e) {
      // Actions warn and continue: one failed click must not lose a page that is
      // otherwise readable.
      return done(false, e);
    }
  }

  async function scriptingWaitSelector(tabId: number, selector: string, timeoutMs = 20000) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const found = await inject(tabId, (sel: string) => !!document.querySelector(sel), [selector]).catch(
        () => false,
      );
      if (found) return { type: 'wait_selector', ok: true };
      await sleep(250);
    }
    throw new Error(`selector ${selector} did not appear`);
  }

  async function scriptingScreenshot(
    tabId: number,
    out: FetchResult,
    actionsRun: NonNullable<FetchResult['actions_run']>,
  ) {
    try {
      await browser.tabs.update(tabId, { active: true });
      // captureVisibleTab captures the ACTIVE tab of a window, so a human
      // switching tabs between the two calls would put an unrelated page on the
      // wire. Re-read the active tab and refuse rather than screenshot the wrong
      // thing.
      const [active] = await browser.tabs.query({ active: true, lastFocusedWindow: true });
      if (!active || active.id !== tabId) {
        actionsRun.push({ type: 'screenshot', ok: false, error: 'another tab became active; screenshot skipped' });
        return;
      }
      const dataUrl = await browser.tabs.captureVisibleTab({ format: 'png' });
      const b64 = dataUrl.slice(dataUrl.indexOf(',') + 1);
      if (b64.length > LIMIT_SHOT_B64) {
        // A frame over the server's read limit closes the socket, which would
        // take the whole fetch down with it. Drop the image and say so.
        actionsRun.push({
          type: 'screenshot',
          ok: false,
          error: `screenshot too large (${b64.length} bytes); use full_page: false`,
        });
        return;
      }
      out.screenshot_b64 = b64;
      actionsRun.push({ type: 'screenshot', ok: true });
    } catch (e) {
      actionsRun.push({ type: 'screenshot', ok: false, error: String((e as any)?.message || e) });
    }
  }

  function toTuple<T>(promise: Promise<T>): Promise<[true, undefined] | [false, unknown]> {
    return promise.then<[true, undefined]>(() => [true, undefined]).catch((e) => [false, e] as [false, unknown]);
  }

  // --------------------------------------------------------------- turbo layer

  async function runTurbo(
    req: FetchRequest,
    tabId: number,
    isCancelled: () => boolean,
    origin: string,
  ): Promise<FetchResult> {
    const actionsRun: NonNullable<FetchResult['actions_run']> = [];
    // Firefox has no chrome.debugger: fail with the reason a person can act on
    // rather than "cannot read properties of undefined".
    const dbg = (browser as any).debugger;
    if (!dbg || !dbg.attach) {
      throw new Error('io: turbo needs Chrome (this browser has no debugger API); use io: scripting');
    }
    const target = { tabId };
    await dbg.attach(target, '1.3');
    try {
      const sendCmd = (method: string, params?: Record<string, unknown>) => dbg.sendCommand(target, method, params);
      const evaluate = async (expression: string) => {
        const r = await sendCmd('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
        if (r.exceptionDetails) throw new Error(r.exceptionDetails.text || 'evaluate failed');
        return r.result ? r.result.value : undefined;
      };

      if (req.wait_selector) {
        const waited = await turboWait(
          evaluate,
          `!!document.querySelector(${JSON.stringify(req.wait_selector)})`,
          req.timeout_ms,
          'wait_selector',
        );
        if (!waited.ok) throw new Error(`selector ${req.wait_selector} did not appear`);
      }

      for (const action of req.actions || []) {
        const stop = stopReason(isCancelled(), paused);
        if (stop) return { status: stop, io: 'turbo', actions_run: actionsRun, message: stop };
        await assertSameOrigin(tabId, origin);
        actionsRun.push(await runTurboAction(sendCmd, evaluate, action, req.timeout_ms, origin));
      }

      // scroll_collect and settle run in the page, through CDP. Same functions
      // as the scripting layer: one definition of "collected" and "settled".
      let collected: FetchResult['collected'];
      if (req.scroll_collect) {
        const raw = await evaluate(`(${scrollAndCollect.toString()})(${JSON.stringify(req.scroll_collect)})`);
        collected = raw as FetchResult['collected'];
        await assertSameOrigin(tabId, origin);
      }

      let settledMs = -1;
      if (req.settle) {
        const raw = await evaluate(`(${settlePage.toString()})(${JSON.stringify(req.settle)})`);
        settledMs = (raw as { ms?: number })?.ms ?? -1;
      } else {
        await sleep(req.settle_ms || 300);
      }
      await assertSameOrigin(tabId, origin);

      // Same fields, same caps as the scripting layer: readPage() is the single
      // source of that contract, serialised the same way.
      const page = (await evaluate(
        `(${readPage.toString()})(${LIMIT_HTML},${LIMIT_TEXT})`,
      )) as {
        url: string;
        title: string;
        text: string;
        html: string;
        truncated: boolean;
        status: number;
      };
      const out: FetchResult = {
        status: 'ok',
        io: 'turbo',
        actions_run: actionsRun,
        settled_ms: settledMs,
        collected,
        final_url: page.url,
        title: page.title,
        text: page.text,
        html: page.html,
        html_truncated: page.truncated,
      };
      if (page.status) out.http_status = page.status;

      const markers = detectMarkers(page, BUILTIN_MARKERS);
      if (markers.length) {
        out.status = 'awaiting-human';
        out.markers = markers;
      }

      if (req.page_tree === 'ax' || req.page_tree === 'both') {
        out.tree = await turboA11yTree(sendCmd);
      }
      if (req.page_tree === 'dom') {
        out.tree = (await evaluate(
          `(${pageTreeDom.toString()})(${JSON.stringify(req.page_tree_options || {})})`,
        )) as FetchResult['tree'];
      }
      if (req.motion) {
        out.motion = (await evaluate(`(${pageMotion.toString()})()`)) as FetchResult['motion'];
      }

      if (req.screenshot) {
        try {
          const shot = await sendCmd('Page.captureScreenshot', {
            format: 'png',
            captureBeyondViewport: !!req.full_page,
          });
          if (shot.data && shot.data.length > LIMIT_SHOT_B64) {
            actionsRun.push({
              type: 'screenshot',
              ok: false,
              error: `screenshot too large (${shot.data.length} bytes); use full_page: false`,
            });
          } else {
            out.screenshot_b64 = shot.data;
            actionsRun.push({ type: 'screenshot', ok: true });
          }
        } catch (e) {
          actionsRun.push({ type: 'screenshot', ok: false, error: String((e as any)?.message || e) });
        }
      }
      return out;
    } finally {
      try {
        await dbg.detach(target);
      } catch {
        /* already gone */
      }
    }
  }

  // turboA11yTree reads the real accessibility tree and folds the flat node
  // list into something a model can walk. This is the layer that knows a nav is
  // a navigation and a carousel is a list, which the DOM outline can only
  // guess at.
  async function turboA11yTree(sendCmd: (m: string, p?: Record<string, unknown>) => Promise<any>) {
    await sendCmd('Accessibility.enable');
    const { nodes } = await sendCmd('Accessibility.getFullAXTree');
    const byId = new Map<string, any>();
    for (const n of nodes || []) byId.set(n.nodeId, n);
    const seen = new Set<string>();
    let budget = 1200;

    const walk = (node: any, depth: number): any => {
      if (!node || seen.has(node.nodeId) || budget <= 0 || depth > 16) return null;
      seen.add(node.nodeId);
      budget--;
      const role = node.role?.value || 'generic';
      const name = node.name?.value || '';
      if (node.ignored && role === 'generic' && !name) return null;
      const kids: any[] = [];
      for (const childId of node.childIds || []) {
        const child = walk(byId.get(childId), depth + 1);
        if (child) kids.push(child);
      }
      const out: Record<string, unknown> = { tag: role, role, path: role, text_len: name.length, visible: !node.ignored };
      if (name) out.text = name.slice(0, 200);
      if (kids.length) out.children = kids;
      return out;
    };

    const root = (nodes || []).find((n: any) => !n.parentId) || nodes?.[0];
    return walk(root, 0);
  }

  async function runTurboAction(
    sendCmd: (m: string, p?: Record<string, unknown>) => Promise<any>,
    evaluate: (e: string) => Promise<any>,
    action: Action,
    timeoutMs: number | undefined,
    origin: string,
  ) {
    const done = (ok: boolean, error?: unknown) => ({
      type: action.type,
      ok,
      error: error ? String((error as any)?.message || error) : undefined,
    });
    try {
      switch (action.type) {
        case 'wait_ms':
          await sleep(action.ms || 1000);
          return done(true);
        case 'wait_selector':
          return turboWait(
            evaluate,
            `!!document.querySelector(${JSON.stringify(action.selector)})`,
            timeoutMs,
            'wait_selector',
          );
        case 'wait_for_function':
          return turboWait(evaluate, `(${action.script})`, timeoutMs, 'wait_for_function');
        case 'evaluate': {
          await evaluate(`(${action.script})`);
          return done(true);
        }
        case 'scroll_to_bottom':
          await evaluate('window.scrollTo(0, document.body.scrollHeight)');
          return done(true);
        case 'scroll_by':
          await evaluate(`window.scrollBy(0, ${action.px || 800})`);
          return done(true);
        case 'scroll_down':
          await evaluate(`window.scrollBy(0, ${action.px || 0} || window.innerHeight)`);
          return done(true);
        case 'click': {
          const rectRaw = await evaluate(
            `(() => { const el = document.querySelector(${JSON.stringify(action.selector)}); if (!el) return null; el.scrollIntoView({block:'center'}); const r = el.getBoundingClientRect(); return JSON.stringify({x: r.left + r.width/2, y: r.top + r.height/2}); })()`,
          );
          if (!rectRaw) throw new Error(`click: no element for ${action.selector}`);
          const { x, y } = JSON.parse(rectRaw);
          await sleep(150);
          await sendCmd('Input.dispatchMouseEvent', { type: 'mousePressed', x, y, button: 'left', clickCount: 1 });
          await sendCmd('Input.dispatchMouseEvent', { type: 'mouseReleased', x, y, button: 'left', clickCount: 1 });
          return done(true);
        }
        case 'type': {
          const fieldRaw = await evaluate(
            `(() => { const el = document.querySelector(${JSON.stringify(action.selector)}); if (!el) return null; return JSON.stringify({tag: el.tagName, type: el.getAttribute('type'), name: el.getAttribute('name') || el.name, id: el.id, autocomplete: el.getAttribute('autocomplete')}); })()`,
          );
          if (fieldRaw && isCredentialField(JSON.parse(fieldRaw))) {
            return done(
              false,
              `refusing to type into a credential field (${action.selector}): the bridge never enters passwords or codes`,
            );
          }
          await evaluate(
            `(() => { const el = document.querySelector(${JSON.stringify(action.selector)}); if (!el) throw new Error('type: no element'); el.focus(); return true; })()`,
          );
          await sendCmd('Input.insertText', { text: action.text || '' });
          return done(true);
        }
        case 'press': {
          const key = action.key || 'Enter';
          const base = { key, code: key, windowsVirtualKeyCode: keyCode(key), nativeVirtualKeyCode: keyCode(key) };
          const down = key === 'Enter' ? { ...base, text: '\r', unmodifiedText: '\r' } : base;
          await sendCmd('Input.dispatchKeyEvent', { type: 'keyDown', ...down });
          await sendCmd('Input.dispatchKeyEvent', { type: 'keyUp', ...base });
          return done(true);
        }
        case 'navigate': {
          const drift = originDrift(origin, action.url);
          if (drift) return done(false, drift);
          await sendCmd('Page.navigate', { url: action.url });
          await turboWait(evaluate, `document.readyState === 'complete'`, timeoutMs, 'navigate');
          return done(true);
        }
        default:
          return done(false, `unknown action ${action.type}`);
      }
    } catch (e) {
      return done(false, e);
    }
  }

  async function turboWait(
    evaluate: (e: string) => Promise<any>,
    expression: string,
    timeoutMs = 20000,
    label = 'wait',
  ) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const ok = await evaluate(expression).catch(() => false);
      if (ok) return { type: label, ok: true };
      await sleep(250);
    }
    return { type: label, ok: false, error: `${label}: condition not met in ${timeoutMs}ms` };
  }

  function keyCode(key: string) {
    switch (key) {
      case 'Enter':
        return 13;
      case 'Tab':
        return 9;
      case 'Escape':
        return 27;
      default:
        return 0;
    }
  }

  // ---------------------------------------------------------------- lifecycle

  browser.runtime.onStartup.addListener(() => connect());
  browser.runtime.onInstalled.addListener(() => connect());
  browser.storage.onChanged.addListener((changes: any, area: string) => {
    if (area !== 'local') return;
    if (changes.paused) paused = !!changes.paused.newValue;
    const tokenChanged = changes.token && changes.token.newValue !== changes.token.oldValue;
    const portChanged = changes.port && changes.port.newValue !== changes.port.oldValue;
    if (tokenChanged && changes.token.newValue !== ownToken) restartSocket();
    else if (portChanged) restartSocket();
  });

  // A service worker is killed when it goes idle. While the socket is up the
  // server's 20s pings keep it alive, but a window with no Tomoshibi running (or
  // a restarted API) must not leave the bridge silently dead until the user
  // happens to open the popup: the alarm wakes the worker and reconnects.
  async function heartbeat() {
    await browser.alarms.create(HEARTBEAT, { periodInMinutes: 0.5 });
  }
  browser.alarms.onAlarm.addListener((alarm: any) => {
    if (alarm.name === HEARTBEAT) connect();
  });

  (async () => {
    const { paused: wasPaused } = await getSettings();
    paused = wasPaused;
    await heartbeat().catch(() => {
      /* alarms are best effort; the socket itself is the real work */
    });
    connect();
  })();
});
