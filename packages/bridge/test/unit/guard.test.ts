import { describe, expect, test } from 'bun:test';
import {
  detectMarkers,
  isCredentialField,
  matchesHost,
  originDrift,
  sanitizeOrigin,
  stopReason,
} from '../../utils/guard';

describe('isCredentialField', () => {
  // The red line: the bridge drives pages, never accounts. Every entry here is a
  // way the browser tells you "this is a secret" and the bridge must respect.
  test('refuses a password input by type', () => {
    expect(isCredentialField({ tag: 'INPUT', type: 'password' })).toBe(true);
  });

  test('refuses by autocomplete token', () => {
    expect(isCredentialField({ autocomplete: 'current-password' })).toBe(true);
    expect(isCredentialField({ autocomplete: 'new-password' })).toBe(true);
    expect(isCredentialField({ autocomplete: 'one-time-code' })).toBe(true);
    expect(isCredentialField({ autocomplete: 'cc-number' })).toBe(true);
  });

  test('refuses by name or id heuristics', () => {
    expect(isCredentialField({ name: 'user_password' })).toBe(true);
    expect(isCredentialField({ id: 'otp' })).toBe(true);
    expect(isCredentialField({ name: 'totp' })).toBe(true);
    expect(isCredentialField({ name: 'card_cvv' })).toBe(true);
  });

  // The review's finding: a hand-masked text input says nothing in `type` or
  // `autocomplete`, so the name is the only signal left — and `name="pwd"` is
  // what a real form uses.
  test('refuses the names a hand-masked field uses', () => {
    expect(isCredentialField({ type: 'text', name: 'pwd' })).toBe(true);
    expect(isCredentialField({ type: 'text', id: 'passcode' })).toBe(true);
    expect(isCredentialField({ type: 'text', name: 'user_pin' })).toBe(true);
  });

  test('does not treat unrelated words containing "pin" as credentials', () => {
    expect(isCredentialField({ type: 'text', name: 'pinterest_url' })).toBe(false);
    expect(isCredentialField({ type: 'text', name: 'pinned' })).toBe(false);
  });

  test('allows an ordinary search box', () => {
    expect(isCredentialField({ tag: 'INPUT', type: 'search', name: 'q' })).toBe(false);
    expect(isCredentialField({ type: 'text', name: 'query', id: 'search' })).toBe(false);
  });

  test('an absent field description is not a credential', () => {
    expect(isCredentialField(null)).toBe(false);
    expect(isCredentialField(undefined)).toBe(false);
  });
});

describe('originDrift', () => {
  test('the requested origin is fine', () => {
    expect(originDrift('https://example.com', 'https://example.com/a/b')).toBe('');
  });

  test('a cross-origin landing is refused and named', () => {
    const msg = originDrift('https://example.com', 'https://evil.test/x');
    expect(msg).toContain('left https://example.com');
    expect(msg).toContain('https://evil.test');
  });

  test('a subdomain is a different origin', () => {
    expect(originDrift('https://example.com', 'https://www.example.com/')).not.toBe('');
  });

  test('a missing final URL is not a drift', () => {
    expect(originDrift('https://example.com', '')).toBe('');
    expect(originDrift('https://example.com', undefined)).toBe('');
  });

  test('an unparseable URL is refused rather than waved through', () => {
    expect(originDrift('https://example.com', 'not a url')).toContain('unexpected final URL');
  });
});

describe('stopReason', () => {
  test('cancelled wins over paused', () => {
    expect(stopReason(true, true)).toBe('cancelled');
    expect(stopReason(true, false)).toBe('cancelled');
  });

  test('pause interrupts a run that is still wanted', () => {
    expect(stopReason(false, true)).toBe('paused');
    expect(stopReason(false, false)).toBe('');
  });
});

describe('matchesHost', () => {
  test('exact, wildcard and cleaned input', () => {
    expect(matchesHost(['example.com'], 'example.com')).toBe(true);
    expect(matchesHost(['*.example.com'], 'docs.example.com')).toBe(true);
    expect(matchesHost(['*.example.com'], 'example.com')).toBe(true);
    expect(matchesHost(['https://example.com:8443/x'], 'example.com')).toBe(true);
    expect(matchesHost(['example.com'], 'www.example.com')).toBe(false);
    expect(matchesHost([], 'example.com')).toBe(false);
  });
});

describe('detectMarkers', () => {
  test('finds a hint in the title or the text', () => {
    expect(detectMarkers({ title: 'Just a moment...' }, ['Just a moment...'])).toEqual(['Just a moment...']);
    expect(detectMarkers({ text: 'please enable javascript and cookies' }, ['nope'])).toEqual([]);
  });
});

describe('sanitizeOrigin', () => {
  test('keeps hosts, drops anything else', () => {
    expect(sanitizeOrigin('https://Google.COM/search?q=x')).toBe('google.com');
    expect(sanitizeOrigin('*.example.com')).toBe('*.example.com');
    expect(sanitizeOrigin('not a host')).toBe('');
    expect(sanitizeOrigin('')).toBe('');
  });
});
