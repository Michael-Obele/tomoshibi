import type { RequestHandler } from './$types';
import { llmsFull, links, respond } from '$lib/server/agentdocs';

export const GET: RequestHandler = ({ request }) =>
	respond(request, llmsFull, 'text/plain; charset=utf-8', links.describedBy);
