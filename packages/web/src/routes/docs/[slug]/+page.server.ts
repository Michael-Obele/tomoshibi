import { error } from '@sveltejs/kit';
import { manifest } from '$lib/server/agentdocs';
import { htmlFor } from '$lib/server/docs-html';

export function load({ params }) {
	const doc = manifest.docs.find((candidate) => candidate.slug === params.slug);
	const html = doc ? htmlFor(params.slug) : undefined;

	if (!doc || html === undefined) {
		error(404, 'Page not found');
	}

	return { doc, html };
}
