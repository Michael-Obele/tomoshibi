import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { docs } from './vite-plugin-docs';

export default defineConfig({
    plugins: [tailwindcss(), sveltekit(), docs()],
    ssr: {
        noExternal: ['svelte-idb']
    }
});
