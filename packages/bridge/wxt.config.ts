import { defineConfig } from 'wxt';

// One codebase, two browsers. The only real divergence is `debugger`: it is
// what powers the turbo layer, it is Chrome-only, and asking for it in a
// Firefox manifest is a permission a user should never be shown.
export default defineConfig({
  modules: ['@wxt-dev/module-svelte'],
  // Firefox MV3, not MV2: WXT still defaults Firefox to MV2, which would
  // ship a different extension than the one the plan and the tests describe.
  manifestVersion: 3,
  // es2022 keeps async/await and class fields native. This matters more than it
  // looks: `scripting.executeScript` serialises a function with toString(), so
  // any transpiler helper (__async, __spreadArray) would be a dangling
  // reference inside the page.
  vite: () => ({ build: { target: 'es2022' } }),
  manifest: ({ browser }) => ({
    name: 'Tomoshibi Bridge',
    description:
      'Lets your local Tomoshibi read and drive pages in the browser you are already signed into. Loopback only; pairs with one code.',
    permissions:
      browser === 'firefox'
        ? ['tabs', 'scripting', 'storage', 'alarms']
        : ['tabs', 'scripting', 'storage', 'debugger', 'alarms'],
    host_permissions: ['<all_urls>'],
    ...(browser === 'firefox'
      ? {
          browser_specific_settings: {
            gecko: { id: 'bridge@tomoshibi.dev', strict_min_version: '128.0' },
          },
        }
      : {}),
  }),
});
