// Shared browser glue for the E2E suites: which Chrome to use, a tiny CDP
// client, and the extension-id derivation.
//
// WHY THE BROWSER CHOICE MATTERS
//
// Branded Chrome 137+ ignores --load-extension for automation, and the CDP
// replacement (Extensions.loadUnpacked) only half-works: the extension is
// registered (it even shows up in the profile's Preferences) but Chrome never
// starts its service worker and every chrome-extension:// page answers
// ERR_BLOCKED_BY_CLIENT. Verified on Google Chrome 157.0.8103.50, with and
// without --enable-unsafe-extension-debugging.
//
// Chrome for Testing (or Chromium) still honours --load-extension, and the worker
// comes up on the next tick. Playwright keeps a CfT build in the cache, which is
// the build these tests use unless CHROME_BIN says otherwise.

import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

export function resolveChrome(): string {
  if (process.env.CHROME_BIN && existsSync(process.env.CHROME_BIN)) return process.env.CHROME_BIN;
  const root = join(process.env.HOME!, '.cache/ms-playwright');
  if (existsSync(root)) {
    const builds = readdirSync(root)
      .filter((d) => d.startsWith('chromium-'))
      .sort()
      .reverse();
    for (const d of builds) {
      for (const layout of ['chrome-linux64', 'chrome-linux']) {
        const bin = join(root, d, layout, 'chrome');
        if (existsSync(bin)) return bin;
      }
    }
  }
  throw new Error(
    'no Chrome for Testing build found (set CHROME_BIN to a Chromium/CfT binary; branded Chrome will not load the extension)',
  );
}

type Pend = { resolve: (v: any) => void; reject: (e: Error) => void };

export class CDP {
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
    const r = await this.send(
      'Runtime.evaluate',
      { expression, returnByValue: true, awaitPromise: true },
      sessionId,
    );
    if (r.exceptionDetails) {
      throw new Error(
        `evaluate: ${r.exceptionDetails.text} ${r.exceptionDetails.exception?.description || ''}`,
      );
    }
    return r.result?.value;
  }

  close() {
    try {
      this.ws.close();
    } catch {
      /* gone */
    }
  }
}

/** The extension id Chrome derives from the unpacked directory's absolute path. */
export function extensionIdFromPath(path: string): string {
  const hash = new Bun.CryptoHasher('sha256').update(path).digest();
  let id = '';
  for (let i = 0; i < 16; i++) {
    const byte = hash[i] ?? 0;
    id += String.fromCharCode(97 + (byte >> 4));
    id += String.fromCharCode(97 + (byte & 0xf));
  }
  return id;
}

export async function waitFor(
  fn: () => Promise<boolean> | boolean,
  timeoutMs: number,
  what: string,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await fn()) return;
    await Bun.sleep(300);
  }
  throw new Error(`timeout waiting for ${what}`);
}
