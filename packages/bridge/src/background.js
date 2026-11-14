// Tomoshibi Bridge — service worker.
//
// One job: keep a loopback WebSocket to the local Tomoshibi API, and when it
// asks, read or drive a page inside this browser (the one you are signed into)
// and return what the page shows. Two IO layers:
//   scripting (default) — DOM events through the extension scripting API.
//                         No debugger, no infobar, works on Firefox.
//   turbo               — chrome.debugger CDP: trusted input and full-page
//                         screenshots. Chrome only; shows the debugging infobar.
//
// It never types credentials, never exports cookies, never reads tabs it was
// not asked about. Everything it does lands in Tomoshibi's access log.

import { PROTOCOL, DEFAULT_PORT, BUILTIN_MARKERS, sleep, api, getSettings } from './lib.js';

let ws = null;
let reconnectTimer = null;
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
const tabsByOrigin = new Map(); // origin -> tabId (one pinned tab per origin)
const cancelled = new Set(); // fetch ids the server cancelled
let queue = Promise.resolve(); // serializes runs (v1: one at a time)
const HEARTBEAT = 'tomoshibi-bridge-heartbeat';

// ---------------------------------------------------------------- connection

function badge(text, color) {
  try {
    api.action.setBadgeText({ text });
    if (color) api.action.setBadgeBackgroundColor({ color });
  } catch {
    /* badge is cosmetic; never let it break the bridge */
  }
}

function send(msg) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(msg));
    return true;
  }
  return false;
}

async function connect() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
  const { port } = await getSettings();
  let socket;
  try {
    socket = new WebSocket(`ws://127.0.0.1:${port}/bridge`);
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
    // proved it knows the key we saved at pairing time (see onChallenge).
    verified = false;
    send({
      type: 'hello',
      protocol: PROTOCOL,
      browser: navigator.userAgent.includes('Firefox') ? 'firefox' : 'chrome',
      version: api.runtime.getManifest().version,
    });
    broadcast();
  };

  socket.onmessage = (ev) => {
    if (socket !== ws) return;
    let msg;
    try {
      msg = JSON.parse(ev.data);
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
  clearTimeout(reconnectTimer);
  reconnectTimer = setTimeout(connect, reconnectDelay);
  reconnectDelay = Math.min(reconnectDelay * 2, 30000);
}

// Drop the current socket and come back promptly with fresh settings.
// Deliberate changes (new port, revoked token) must not wait out a backoff that
// has already grown to 30s, and `close()` on an already-closed socket fires no
// event at all, so the reconnect is scheduled here rather than hoped for.
function restartSocket(delay = 150) {
  reconnectDelay = 2000;
  const old = ws;
  ws = null;
  clearTimeout(reconnectTimer);
  if (old) {
    try {
      old.close();
    } catch {
      /* already gone */
    }
  }
  reconnectTimer = setTimeout(connect, delay);
}

// ------------------------------------------------------------- handshake

// hmacHex is the extension's half of the pairing handshake: HMAC-SHA256 in the
// exact shape the server uses, so both sides can check each other.
async function hmacHex(key, message) {
  const enc = new TextEncoder();
  const k = await crypto.subtle.importKey(
    'raw',
    enc.encode(key),
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign'],
  );
  const sig = await crypto.subtle.sign('HMAC', k, enc.encode(message));
  return [...new Uint8Array(sig)].map((b) => b.toString(16).padStart(2, '0')).join('');
}

// challenge answers hello. If we hold a server key we verify the proof before
// sending the token; if we do not (never paired), there is nothing to release
// and nothing to verify, so we wait to be told whether pairing is needed.
async function onChallenge(msg) {
  lastNonce = msg.nonce;
  const { token, serverKey } = await getSettings();
  if (!serverKey) return; // unpaired: the token slot is empty anyway
  const want = await hmacHex(serverKey, `tomoshibi-srv|${msg.nonce}`);
  if (want !== msg.proof) {
    // Something else is answering the port, or the pairing was revoked and the
    // server minted a new key. Either way: no token, no fetches. The socket
    // stays open so the user can pair again from the popup — pairing is
    // verified against the code they type, which is the only proof available
    // before any key is shared.
    verified = false;
    paired = false;
    lastError = `unverified server on port ${(await getSettings()).port}: it does not hold our pairing key. If Tomoshibi was reinstalled or its state deleted, pair again.`;
    badge('err', '#b91c1c');
    broadcast();
    return;
  }
  verified = true;
  send({ type: 'auth', token, nonce: msg.nonce });
}

async function handleFrame(msg) {
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
      // Verify the server knew the code the user typed. Without this, whatever
      // answered the port could hand us a token and claim a successful pairing.
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
        if (ws) try { ws.close(); } catch { /* reconnect */ }
        break;
      }
      // Remember what we wrote so the storage echo below is not mistaken for a
      // settings change (which would close the socket we just paired on).
      ownToken = msg.token;
      await api.storage.local.set({ token: msg.token, serverKey: msg.server_key });
      verified = true;
      paired = true;
      lastError = '';
      badge('on', '#15803d');
      broadcast();
      break;
    }
    case 'status':
      paired = !!msg.paired && verified;
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
        send({ type: 'fetch.result', id: msg.id, status: 'error', message: 'extension is not paired with this server' });
        break;
      }
      queueRun(msg);
      break;
    case 'cancel':
      cancelled.add(msg.id);
      break;
    default:
      break;
  }
}

function queueRun(req) {
  queue = queue
    .then(() => runFetch(req))
    .catch((e) => {
      send({ type: 'fetch.result', id: req.id, status: 'error', message: String((e && e.message) || e) });
    });
}

// ---------------------------------------------------------------- popup API

api.runtime.onMessage.addListener((msg, _sender, reply) => {
  (async () => {
    switch (msg.kind) {
      case 'status': {
        const { port } = await getSettings();
        reply({ connected, paired, paused, lastError, port });
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
        // which mints a new server key. Without this the extension would hold a
        // key the server no longer has and never authenticate again.
        ownToken = '';
        verified = false;
        paired = false;
        lastError = '';
        await api.storage.local.remove(['token', 'serverKey']);
        restartSocket();
        reply({ forgotten: true });
        break;
      case 'pause':
        paused = !!msg.on;
        await api.storage.local.set({ paused });
        broadcast();
        reply({ paused });
        break;
      case 'port':
        await api.storage.local.set({ port: Number(msg.port) || DEFAULT_PORT });
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
  api.runtime
    .sendMessage({ kind: 'state', connected, paired, paused, lastError })
    .catch(() => {
      /* no popup open */
    });
}

// ---------------------------------------------------------------- fetch run

// Page read caps, shared by both layers: one place to change, so the two
// implementations cannot disagree about how much of a page comes back.
const LIMIT_HTML = 8_000_000;
const LIMIT_TEXT = 2_000_000;
// A base64 PNG larger than this is dropped rather than sent: the server's frame
// limit has to hold HTML, text and the image at once, and an over-limit frame
// closes the socket instead of failing one fetch.
const LIMIT_SHOT_B64 = 8_000_000;

async function runFetch(req) {
  const started = Date.now();
  const reply = (extra) =>
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
    const io = req.io === 'turbo' ? 'turbo' : 'scripting';
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
    reply(out);
  } catch (e) {
    const message = String((e && e.message) || e);
    // "paused" is a distinct outcome, not an error: the server reports it as
    // the paused code rather than a failure to fetch.
    reply({ status: message === 'paused' ? 'paused' : 'error', message });
    cancelled.delete(req.id);
  }
}

// originDrift returns a message when the page ended up somewhere the request
// did not ask for, and "" when the final URL still belongs to the request's
// origin. Both arguments are origins (or a URL whose origin is taken).
function originDrift(wantOrigin, finalUrl) {
  if (!finalUrl) return '';
  let got;
  try {
    got = new URL(finalUrl).origin;
  } catch {
    return `unexpected final URL ${finalUrl}`;
  }
  if (wantOrigin === got) return '';
  return `page left ${wantOrigin} for ${got} (redirected or navigated); refusing to drive or read another origin`;
}

// assertSameOrigin throws when the tab has left the origin the fetch was
// allowed to drive. Called between actions, not only at the end: by the time
// the read-back happens, the typing has already happened.
async function assertSameOrigin(tabId, wantOrigin) {
  let tab;
  try {
    tab = await api.tabs.get(tabId);
  } catch {
    throw new Error('tab closed');
  }
  const drift = originDrift(wantOrigin, tab && tab.url);
  if (drift) throw new Error(drift);
}

// One pinned tab per origin, reused across fetches, left open on a handoff so
// the human can clear whatever it is and retry.
async function tabFor(rawUrl, timeoutMs) {
  const origin = new URL(rawUrl).origin;
  const existingId = tabsByOrigin.get(origin);
  if (existingId != null) {
    try {
      await api.tabs.get(existingId);
      await api.tabs.update(existingId, { url: rawUrl, active: true });
      await waitTabComplete(existingId, timeoutMs);
      return existingId;
    } catch {
      // The pinned tab is unusable (closed, or it never finished loading).
      // Drop it rather than leaving it in the window for the next fetch to
      // duplicate.
      tabsByOrigin.delete(origin);
      try {
        await api.tabs.remove(existingId);
      } catch {
        /* already gone */
      }
    }
  }
  const tab = await api.tabs.create({ url: rawUrl, active: true });
  tabsByOrigin.set(origin, tab.id);
  await waitTabComplete(tab.id, timeoutMs);
  return tab.id;
}

async function waitTabComplete(tabId, timeoutMs = 45000) {
  const deadline = Date.now() + timeoutMs;
  // Give the navigation a moment to leave the previous 'complete' state.
  await sleep(150);
  while (Date.now() < deadline) {
    let tab;
    try {
      tab = await api.tabs.get(tabId);
    } catch {
      throw new Error('tab closed');
    }
    if (!tab) throw new Error('tab closed');
    if (tab.status === 'complete') return;
    await sleep(200);
  }
  throw new Error('tab did not finish loading in time');
}

// ---------------------------------------------------------- scripting layer

async function runScripting(req, tabId, isCancelled, origin) {
  const out = { status: 'ok', io: 'scripting', actions_run: [] };

  if (req.wait_selector) {
    // The request-level wait is the recipe's assertion that the page it wants
    // has loaded. It is fatal on both layers: returning a page that never
    // rendered is the silent failure the recipes exist to prevent. Inside
    // `actions`, a wait stays advisory (a missed click should not lose a page).
    await scriptingWaitSelector(tabId, req.wait_selector, req.timeout_ms);
  }
  for (const action of req.actions || []) {
    const stop = stopReason(isCancelled);
    if (stop) {
      out.status = stop;
      out.message = stop;
      return out;
    }
    // Between actions, not just at the end: by the time the read-back happens
    // the typing has already happened.
    await assertSameOrigin(tabId, origin);
    out.actions_run.push(await runScriptingAction(tabId, action, req.timeout_ms, origin));
  }

  await sleep(req.settle_ms || 300);
  await assertSameOrigin(tabId, origin);
  // The caps are passed as args: an injected function is serialised, so it
  // cannot close over module constants.
  const page = await inject(tabId, readPage, [LIMIT_HTML, LIMIT_TEXT]);
  if (!page) throw new Error('could not read the page');

  out.final_url = page.url;
  out.title = page.title;
  out.text = page.text;
  out.html = page.html;
  out.html_truncated = page.truncated;
  if (page.status) out.http_status = page.status;

  const markers = detectMarkers(page);
  if (markers.length) {
    out.status = 'awaiting-human';
    out.markers = markers;
  }

  if (req.screenshot) {
    await scriptingScreenshot(tabId, out);
  }
  return out;
}

async function runScriptingAction(tabId, action, timeoutMs, origin) {
  const done = (ok, error) => ({ type: action.type, ok, error: error ? String((error && error.message) || error) : undefined });
  try {
    switch (action.type) {
      case 'wait_ms':
        await sleep(action.ms || 1000);
        return done(true);
      case 'wait_selector':
        return done(...(await toTuple(scriptingWaitSelector(tabId, action.selector, timeoutMs))));
      case 'wait_for_function':
        return done(false, 'wait_for_function requires io: turbo');
      case 'evaluate':
        return done(false, 'evaluate requires io: turbo');
      case 'scroll_to_bottom':
        await inject(tabId, () => window.scrollTo(0, document.body.scrollHeight));
        return done(true);
      case 'scroll_by':
        await inject(tabId, (px) => window.scrollBy(0, px), [action.px || 800]);
        return done(true);
      case 'scroll_down':
        await inject(tabId, (px) => window.scrollBy(0, px || window.innerHeight), [action.px || 0]);
        return done(true);
      case 'click':
        await inject(tabId, clickBySelector, [action.selector]);
        return done(true);
      case 'type': {
        // The bridge drives pages, never accounts: a password, one-time code or
        // card field is refused outright rather than trusted to the recipe.
        const field = await inject(tabId, readFieldAttrs, [action.selector]);
        if (field && !field.missing && isCredentialField(field)) {
          return done(false, `refusing to type into a credential field (${action.selector}): the bridge never enters passwords or codes`);
        }
        await inject(tabId, typeIntoSelector, [action.selector, action.text || '']);
        return done(true);
      }
      case 'press':
        await inject(tabId, pressKey, [action.key || 'Enter']);
        return done(true);
      case 'navigate': {
        // A navigate that leaves the origin would be driving another site with
        // the user's session; the recipe declares the host it may touch.
        const drift = originDrift(origin, action.url);
        if (drift) return done(false, drift);
        await api.tabs.update(tabId, { url: action.url });
        await waitTabComplete(tabId, timeoutMs);
        return done(true);
      }
      default:
        return done(false, `unknown action ${action.type}`);
    }
  } catch (e) {
    // Actions warn and continue, matching Tomoshi's policy: one failed click
    // must not lose a page that is otherwise readable.
    return done(false, e);
  }
}

async function scriptingWaitSelector(tabId, selector, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const found = await inject(tabId, (sel) => !!document.querySelector(sel), [selector]).catch(() => false);
    if (found) return { type: 'wait_selector', ok: true };
    await sleep(250);
  }
  throw new Error(`selector ${selector} did not appear`);
}

async function scriptingScreenshot(tabId, out) {
  try {
    await api.tabs.update(tabId, { active: true });
    // captureVisibleTab captures the ACTIVE tab of a window, so a human
    // switching tabs between the two calls would put an unrelated page on the
    // wire. Re-read the active tab and refuse rather than screenshot the wrong
    // thing.
    const [active] = await api.tabs.query({ active: true, lastFocusedWindow: true });
    if (!active || active.id !== tabId) {
      return { type: 'screenshot', ok: false, error: 'another tab became active; screenshot skipped' };
    }
    const dataUrl = await api.tabs.captureVisibleTab(undefined, { format: 'png' });
    const b64 = dataUrl.slice(dataUrl.indexOf(',') + 1);
    if (b64.length > LIMIT_SHOT_B64) {
      // A frame over the server's read limit closes the socket, which would
      // take the whole fetch down with it. Drop the image and say so.
      return {
        type: 'screenshot', ok: false,
        error: `screenshot too large (${b64.length} bytes); use full_page: false`,
      };
    }
    out.screenshot_b64 = b64;
    return { type: 'screenshot', ok: true };
  } catch (e) {
    return { type: 'screenshot', ok: false, error: String((e && e.message) || e) };
  }
}

function toTuple(promise) {
  return promise.then((v) => [true]).catch((e) => [false, e]);
}

// Functions below are serialized and injected into the page; they must be
// self-contained (no imports, no closures) except for the values passed as
// args. readPageExpression() is the string the turbo layer evaluates, so both
// layers read exactly the same fields with exactly the same caps.
// stopReason is the reason a run must not continue: the server gave up
// (cancelled) or the user flipped the pause switch mid-run. The popup switch
// has to interrupt the next action, not the next fetch.
function stopReason(isCancelled) {
  if (isCancelled()) return 'cancelled';
  if (paused) return 'paused';
  return '';
}

function readPageExpression() {
  return `JSON.stringify({url: location.href, title: document.title, text: (document.body?document.body.innerText:'').slice(0,${LIMIT_TEXT}), html: (document.documentElement?document.documentElement.outerHTML:'').slice(0,${LIMIT_HTML}), truncated: (document.documentElement?document.documentElement.outerHTML:'').length > ${LIMIT_HTML}, status: ((performance.getEntriesByType('navigation')||[])[0]||{}).responseStatus||0})`;
}

function readPage(htmlLimit, textLimit) {
  const html = document.documentElement ? document.documentElement.outerHTML : '';
  // The navigation's HTTP status when the browser will report it (same-origin
  // navigations do; cross-origin ones may not), 0 otherwise. A recipe's
  // expect.status only fires on a reported value.
  const nav = (performance.getEntriesByType('navigation') || [])[0];
  return {
    url: location.href,
    title: document.title,
    text: (document.body ? document.body.innerText : '').slice(0, textLimit),
    html: html.slice(0, htmlLimit),
    truncated: html.length > htmlLimit,
    status: (nav && nav.responseStatus) || 0,
  };
}

function clickBySelector(selector) {
  const el = document.querySelector(selector);
  if (!el) throw new Error(`click: no element for ${selector}`);
  el.scrollIntoView({ block: 'center', inline: 'center' });
  const rect = el.getBoundingClientRect();
  const x = rect.left + rect.width / 2;
  const y = rect.top + rect.height / 2;
  const opts = { bubbles: true, cancelable: true, view: window, clientX: x, clientY: y, button: 0 };
  el.dispatchEvent(new PointerEvent('pointerdown', opts));
  el.dispatchEvent(new MouseEvent('mousedown', opts));
  el.dispatchEvent(new PointerEvent('pointerup', opts));
  el.dispatchEvent(new MouseEvent('mouseup', opts));
  el.dispatchEvent(new MouseEvent('click', opts));
}

function typeIntoSelector(selector, text) {
  const el = document.querySelector(selector);
  if (!el) throw new Error(`type: no element for ${selector}`);
  el.focus();
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
  if (setter) setter.call(el, text);
  else el.value = text;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

// isCredentialField reports whether a field description is one the bridge must
// never type into (see the plan's red line: no code path types credentials).
// The scripting layer asks the page for the attributes and decides here,
// because injected functions are serialised and cannot see this one.
function isCredentialField(field) {
  if (!field) return false;
  const type = String(field.type || '').toLowerCase();
  if (type === 'password') return true;
  const ac = String(field.autocomplete || '').toLowerCase();
  if (ac.includes('current-password') || ac.includes('new-password')) return true;
  if (ac.includes('one-time-code')) return true;
  if (ac.startsWith('cc-')) return true;
  const name = `${field.name || ''} ${field.id || ''}`.toLowerCase();
  return /password|passwd|otp|one-?time|2fa|mfa|totp|cvv|cvc/.test(name);
}

function readFieldAttrs(selector) {
  const el = document.querySelector(selector);
  if (!el) return { missing: true };
  return {
    tag: el.tagName,
    type: el.getAttribute('type'),
    name: el.getAttribute('name') || el.name,
    id: el.id,
    autocomplete: el.getAttribute('autocomplete'),
  };
}

function pressKey(key) {
  const el = document.activeElement || document.body;
  const opts = { key, code: key, bubbles: true, cancelable: true };
  const down = new KeyboardEvent('keydown', opts);
  const notPrevented = el.dispatchEvent(down);
  el.dispatchEvent(new KeyboardEvent('keyup', opts));
  // Synthetic events do not run browser defaults, so Enter's submit is done by
  // hand. It mirrors the implicit-submission rules a real Enter follows: only
  // when the form has a submit control, or exactly one field. Calling
  // requestSubmit() unconditionally would submit multi-field forms that a
  // human's Enter would not.
  if (key !== 'Enter' || !notPrevented) return;
  const form = el.closest && el.closest('form');
  if (!form || typeof form.requestSubmit !== 'function') return;
  const hasSubmit = !!form.querySelector('input[type=submit], button[type=submit], button:not([type])');
  const singleField = form.elements.length === 1;
  if (hasSubmit || singleField) form.requestSubmit();
}

async function inject(tabId, func, args = []) {
  const results = await api.scripting.executeScript({ target: { tabId }, func, args });
  const first = results && results[0];
  if (!first) throw new Error('script injection returned nothing');
  if (first.error) throw new Error(first.error);
  return first.result;
}

function detectMarkers(page) {
  const haystack = `${page.title}\n${page.text}`;
  return BUILTIN_MARKERS.filter((m) => haystack.includes(m));
}

// --------------------------------------------------------------- turbo layer

async function runTurbo(req, tabId, isCancelled, origin) {
  const out = { status: 'ok', io: 'turbo', actions_run: [] };
  // Firefox has no chrome.debugger: fail with the reason a person can act on
  // rather than "cannot read properties of undefined".
  if (!api.debugger || !api.debugger.attach) {
    throw new Error('io: turbo needs Chrome (this browser has no debugger API); use io: scripting');
  }
  const target = { tabId };
  await api.debugger.attach(target, '1.3');
  try {
    const sendCmd = (method, params) => api.debugger.sendCommand(target, method, params);
    const evaluate = async (expression) => {
      const r = await sendCmd('Runtime.evaluate', { expression, returnByValue: true });
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.text || 'evaluate failed');
      return r.result ? r.result.value : undefined;
    };

    if (req.wait_selector) {
      // Fatal on both layers: see the note in runScripting.
      const waited = await turboWait(evaluate, `!!document.querySelector(${JSON.stringify(req.wait_selector)})`, req.timeout_ms, 'wait_selector');
      if (!waited.ok) throw new Error(`selector ${req.wait_selector} did not appear`);
    }

    for (const action of req.actions || []) {
      const stop = stopReason(isCancelled);
      if (stop) {
        out.status = stop;
        out.message = stop;
        return out;
      }
      await assertSameOrigin(tabId, origin);
      out.actions_run.push(await runTurboAction(sendCmd, evaluate, tabId, action, req.timeout_ms, origin));
    }

    await sleep(req.settle_ms || 300);
    await assertSameOrigin(tabId, origin);
    // Same fields, same caps as the scripting layer: readPageExpression() is
    // the single source of that contract.
    const raw = await evaluate(readPageExpression());
    const page = JSON.parse(raw);
    out.final_url = page.url;
    out.title = page.title;
    out.text = page.text;
    out.html = page.html;
    out.html_truncated = page.truncated;
    if (page.status) out.http_status = page.status;

    const markers = detectMarkers(page);
    if (markers.length) {
      out.status = 'awaiting-human';
      out.markers = markers;
    }

    if (req.screenshot) {
      try {
        const shot = await sendCmd('Page.captureScreenshot', { format: 'png', captureBeyondViewport: !!req.full_page });
        if (shot.data && shot.data.length > LIMIT_SHOT_B64) {
          out.actions_run.push({
            type: 'screenshot', ok: false,
            error: `screenshot too large (${shot.data.length} bytes); use full_page: false`,
          });
        } else {
          out.screenshot_b64 = shot.data;
          out.actions_run.push({ type: 'screenshot', ok: true });
        }
      } catch (e) {
        out.actions_run.push({ type: 'screenshot', ok: false, error: String((e && e.message) || e) });
      }
    }
    return out;
  } finally {
    try {
      await api.debugger.detach(target);
    } catch {
      /* already gone */
    }
  }
}

async function runTurboAction(sendCmd, evaluate, tabId, action, timeoutMs, origin) {
  const done = (ok, error) => ({ type: action.type, ok, error: error ? String((error && error.message) || error) : undefined });
  try {
    switch (action.type) {
      case 'wait_ms':
        await sleep(action.ms || 1000);
        return done(true);
      case 'wait_selector':
        return turboWait(evaluate, `!!document.querySelector(${JSON.stringify(action.selector)})`, timeoutMs, 'wait_selector');
      case 'wait_for_function':
        return turboWait(evaluate, `(${action.script})`, timeoutMs, 'wait_for_function');
      case 'evaluate': {
        const value = await evaluate(`(${action.script})`);
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
          return done(false, `refusing to type into a credential field (${action.selector}): the bridge never enters passwords or codes`);
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

async function turboWait(evaluate, expression, timeoutMs = 20000, label = 'wait') {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const ok = await evaluate(expression).catch(() => false);
    if (ok) return { type: label, ok: true };
    await sleep(250);
  }
  return { type: label, ok: false, error: `${label}: condition not met in ${timeoutMs}ms` };
}

function keyCode(key) {
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

api.runtime.onStartup.addListener(() => connect());
api.runtime.onInstalled.addListener(() => connect());
api.storage.onChanged.addListener((changes, area) => {
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
  await api.alarms.create(HEARTBEAT, { periodInMinutes: 0.5 });
}
api.alarms.onAlarm.addListener((alarm) => {
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
