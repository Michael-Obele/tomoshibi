import type { RequestHandler } from './$types';
import { documents, links, respond } from '$lib/server/agentdocs';

export const GET: RequestHandler = ({ params, request }) => {
	const slug = params.file.replace(/\.md$/, '');
	// hasOwn, not truthiness: `documents['constructor']` would otherwise resolve
	// through the prototype chain and answer 500 instead of 404.
	const body = Object.hasOwn(documents, slug) ? documents[slug] : undefined;

	if (body === undefined) {
		// Point the caller at the list, not just at the failure.
		return new Response(
			JSON.stringify({
				detail: 'request /docs for the list of available slugs',
				docs: '/docs',
				error: `no document "${params.file}"`,
				llms: '/llms.txt'
			}),
			{ status: 404, headers: { 'content-type': 'application/json; charset=utf-8' } }
		);
	}

	return respond(request, body, 'text/markdown; charset=utf-8', links.describedBy);
};
