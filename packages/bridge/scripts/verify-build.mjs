// Verify the built extension is the thing we think it is.
//
// This is the check that would have caught every extension bug this repo has
// actually had:
//   - WXT defaulting Firefox to MV2 while the tests and the plan describe MV3
//   - a manifest version that drifted from package.json after a bump
//   - a permission creeping in (the README promises there is no cookie access)
//   - a build that produced no zip to install
//
// It runs in CI on every PR and on a `bridge-v*` tag, and locally after
// `bun run zip`, so the same assertions hold in both places.

import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const read = (p) => JSON.parse(readFileSync(join(root, p), 'utf8'));

const pkg = read('package.json');
const chrome = read('.output/chrome-mv3/manifest.json');
const firefox = read('.output/firefox-mv3/manifest.json');

const problems = [];
const check = (ok, message) => {
  if (!ok) problems.push(message);
};

check(chrome.version === pkg.version, `chrome manifest ${chrome.version} != package.json ${pkg.version}`);
check(firefox.version === pkg.version, `firefox manifest ${firefox.version} != package.json ${pkg.version}`);
check(chrome.manifest_version === 3, 'chrome build is not MV3');
check(
  firefox.manifest_version === 3,
  'firefox build is not MV3 — WXT defaults Firefox to MV2, so `manifestVersion: 3` must stay in wxt.config.ts',
);
check(chrome.background?.service_worker === 'background.js', 'chrome background is not background.js');
check(
  firefox.background?.service_worker === 'background.js' || Array.isArray(firefox.background?.scripts),
  'firefox has no background entry',
);

const forbidden = ['cookies', 'webrequest', 'nativemessaging', 'externally_connectable', 'history', 'bookmarks'];
for (const [name, manifest] of [['chrome', chrome], ['firefox', firefox]]) {
  const perms = [...(manifest.permissions ?? []), ...(manifest.optional_permissions ?? [])].map((p) =>
    String(p).toLowerCase(),
  );
  const bad = perms.filter((p) => forbidden.some((f) => p.includes(f)));
  check(bad.length === 0, `${name} asks for ${bad.join(', ')} — the README promises it never will`);
}

check((chrome.permissions ?? []).includes('debugger'), 'chrome build lost the debugger permission (the turbo layer needs it)');
check(!(firefox.permissions ?? []).includes('debugger'), 'firefox must not ask for debugger: it has no such API');

const out = join(root, '.output');
const zips = existsSync(out) ? readdirSync(out) : [];
for (const target of ['chrome', 'firefox']) {
  check(
    zips.some((z) => z.endsWith(`-${target}.zip`)),
    `no -${target}.zip in .output — run \`bun run zip\` before verifying`,
  );
}

if (problems.length) {
  for (const p of problems) console.error(`✖ ${p}`);
  process.exit(1);
}

const name = zips.find((z) => z.endsWith('-chrome.zip'));
console.log(`✔ bridge ${pkg.version}: MV3 on both targets, version matches, permissions clean`);
console.log(`  artifact: .output/${name}`);
