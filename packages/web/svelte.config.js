import adapterNetlify from '@sveltejs/adapter-netlify';
import adapterNode from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Netlify is the public host; every other build (local, Docker) runs on node, so
// `bun run build` works without a hosting environment set. Netlify writes to its
// own directory: a node-built `build/` must never be publishable by accident.
const adapter = process.env.NETLIFY
	? adapterNetlify({ publish: 'build-netlify' })
	: adapterNode();

/** @type {import('@sveltejs/kit').Config} */
const config = {
	// Consult https://svelte.dev/docs/kit/integrations
	// for more information about preprocessors
	preprocess: vitePreprocess(),
	vitePlugin: {
		inspector: {
			toggleKeyCombo: 'alt-x',
			showToggleButton: 'always',
			toggleButtonPos: 'bottom-right'
		}
	},

	kit: {
		adapter,

		// Docs pages render repo markdown, which can contain raw HTML. No inline
		// script may run without the nonce SvelteKit adds to its own.
		csp: {
			mode: 'auto',
			directives: {
				'script-src': ['self'],
				'object-src': ['none'],
				'base-uri': ['self']
			}
		},

		experimental: {
			remoteFunctions: true
		}
	},
	compilerOptions: {
		experimental: {
			async: true
		}
	}
};

export default config;
