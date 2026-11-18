import type { RequestHandler } from './$types';
import { links, manifest, respond } from '$lib/server/agentdocs';

// The human /docs page lives beside this file; SvelteKit picks whichever matches
// the request's Accept header.
export const GET: RequestHandler = ({ request }) =>
	respond(
		request,
		JSON.stringify(manifest, null, 2) + '\n',
		'application/json; charset=utf-8',
		links.describedBy
	);
