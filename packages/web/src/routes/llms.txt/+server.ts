import type { RequestHandler } from './$types';
import { llmsIndex, links, respond } from '$lib/server/agentdocs';

export const GET: RequestHandler = ({ request }) =>
	respond(request, llmsIndex, 'text/plain; charset=utf-8', links.index);
