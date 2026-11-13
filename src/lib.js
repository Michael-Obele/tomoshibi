// Shared constants and small helpers. No dependencies; this file is part of
// what an auditor reads, so it stays boring.

export const PROTOCOL = 1;
export const DEFAULT_PORT = 7431;

// Built-in challenge hints. A page containing one of these stops the run as
// awaiting-human: the human clears the check in the open tab, then the fetch
// is retried. These are hints, not a solver; recipes may add their own via
// expect.absent (checked server-side).
export const BUILTIN_MARKERS = [
  'Just a moment...',
  'Enable JavaScript and cookies to continue',
  'Checking your browser before accessing',
  'Verifying you are human',
  'Attention Required!',
  'cf-challenge-running',
];

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// api is chrome.* on Chromium and browser.* on Firefox; both expose promises
// in the versions this supports.
export const api = globalThis.browser ?? globalThis.chrome;

export async function getSettings() {
  const {
    token = '',
    serverKey = '',
    port = DEFAULT_PORT,
    paused = false,
  } = await api.storage.local.get(['token', 'serverKey', 'port', 'paused']);
  return { token, serverKey, port, paused };
}
