// Pure decisions the bridge makes about a page. Nothing here touches the
// browser API, so each rule can be tested on its own — and each one is a rule
// the plan refuses to lose.

/** What the page reported about a field, before any typing happens. */
export interface FieldAttrs {
  tag?: string | null;
  type?: string | null;
  name?: string | null;
  id?: string | null;
  autocomplete?: string | null;
}

/**
 * isCredentialField reports whether a field is one the bridge must never type
 * into. The bridge drives pages, never accounts: you are already signed in.
 *
 * The scripting layer asks the page for the attributes and decides out here,
 * because injected functions are serialised and cannot close over this one.
 */
export function isCredentialField(field: FieldAttrs | null | undefined): boolean {
  if (!field) return false;
  const type = String(field.type || '').toLowerCase();
  if (type === 'password') return true;

  const ac = String(field.autocomplete || '').toLowerCase();
  if (ac.includes('current-password') || ac.includes('new-password')) return true;
  if (ac.includes('one-time-code')) return true;
  if (ac.startsWith('cc-')) return true;

  const name = `${field.name || ''} ${field.id || ''}`.toLowerCase();
  // `pwd`, `passcode` and `pin` are the ones a real form uses when it masks a
  // text input by hand, so type and autocomplete say nothing. `\bpin\b` needs
  // the boundaries: "pinterest" and "pinned" are not credentials.
  if (/password|passwd|pwd|passcode|otp|one-?time|2fa|mfa|totp|cvv|cvc/.test(name)) return true;
  // "user_pin" must match, "pinterest_url" and "pinned" must not. `\b` treats
  // `_` as a word character, so the boundary is written out instead.
  return /(?:^|[^a-z0-9])pin(?:[^a-z0-9]|$)/.test(name);
}

/**
 * originDrift returns a message when a page ended up somewhere the request did
 * not ask for, and "" when it is still on the requested origin.
 *
 * Both arguments are origins, or URLs whose origin is taken. Driving on into a
 * different origin, or reading it back as if it were the requested page, is
 * exactly the cross-origin step the bridge must not take quietly.
 */
export function originDrift(wantOrigin: string, finalUrl: string | undefined | null): string {
  if (!finalUrl) return '';
  let got: string;
  try {
    got = new URL(finalUrl).origin;
  } catch {
    return `unexpected final URL ${finalUrl}`;
  }
  if (wantOrigin === got) return '';
  return `page left ${wantOrigin} for ${got} (redirected or navigated); refusing to drive or read another origin`;
}

/**
 * matchesHost reports whether a hostname matches one of the server's allowlist
 * patterns. `*.example.com` matches subdomains and the bare domain;
 * `example.com` matches exactly. Used only to render state in the popup — the
 * server is the authority and enforces the same rule on its side.
 */
export function matchesHost(patterns: readonly string[], host: string): boolean {
  const h = host.toLowerCase();
  for (const raw of patterns) {
    const p = String(raw || '')
      .trim()
      .toLowerCase()
      .replace(/^https?:\/\//, '')
      .replace(/\/.*$/, '')
      .replace(/:\d+$/, '');
    if (!p) continue;
    if (p.startsWith('*.')) {
      const base = p.slice(2);
      if (h === base || h.endsWith(`.${base}`)) return true;
    } else if (h === p) {
      return true;
    }
  }
  return false;
}

/** detectMarkers returns the built-in challenge hints present on a page. */
export function detectMarkers(
  page: { title?: string; text?: string },
  markers: readonly string[],
): string[] {
  const haystack = `${page.title || ''}\n${page.text || ''}`;
  return markers.filter((m) => haystack.includes(m));
}

/**
 * stopReason is why a run must not continue: the server gave up (cancelled) or
 * the user flipped the pause switch. The switch has to interrupt the next
 * action, not the next fetch.
 */
export function stopReason(isCancelled: boolean, isPaused: boolean): '' | 'cancelled' | 'paused' {
  if (isCancelled) return 'cancelled';
  if (isPaused) return 'paused';
  return '';
}

/**
 * sanitizeOrigin normalises user input for the popup's allowance list.
 * Returns "" for anything that is not an http(s) host pattern.
 */
export function sanitizeOrigin(input: string): string {
  const raw = String(input || '').trim().toLowerCase();
  if (!raw) return '';
  const host = raw.replace(/^https?:\/\//, '').replace(/\/.*$/, '').replace(/:\d+$/, '');
  const ok = /^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/;
  return ok.test(host) ? host : '';
}
