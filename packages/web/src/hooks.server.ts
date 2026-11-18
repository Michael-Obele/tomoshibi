import { type Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { globalLimiter, ipLimiter, burstLimiter } from '$lib/server/rate-limiter';
import type { RateLimiterRes } from 'rate-limiter-flexible';

export const handle: Handle = async ({ event, resolve }) => {
	// The docs an agent reads before it has a key: no auth wall, no rate limit.
	if (isAgentDocs(event.url.pathname)) {
		const response = await resolve(event);
		// /docs answers JSON or HTML by Accept, so shared caches must key on it.
		const vary = response.headers.get('vary');
		if (!vary?.toLowerCase().includes('accept')) {
			response.headers.set('vary', vary ? `${vary}, accept` : 'accept');
		}
		return response;
	}

	const password = env.MASTRA_PASSWORD;

	// 1. AUTH BYPASS
	const authCookie = event.cookies.get('mastra_auth');
	const isAuthenticated = password && authCookie === password;

	// Check if authentication headers are correct
	if (authCookie && !isAuthenticated) {
		event.cookies.delete('mastra_auth', { path: '/' });
	}

	if (isAuthenticated) {
		return resolve(event);
	}

	// 2. GLOBAL TRAFFIC CHECK
	try {
		await globalLimiter.consume('global');
	} catch (rej) {
		const res = rej as RateLimiterRes;
		return new Response('Server Busy: Global traffic limit reached. Please try again later.', {
			status: 503,
			headers: { 'Retry-After': String(Math.ceil(res.msBeforeNext / 1000)) }
		});
	}

	// 3. IP FAIRNESS CHECK
	const ip = event.getClientAddress();
	try {
		await ipLimiter.consume(ip);
	} catch (rej) {
		const res = rej as RateLimiterRes;
		return new Response('Rate Limit Exceeded: You are browsing too fast. Please wait 5 minutes.', {
			status: 429,
			headers: { 'Retry-After': String(Math.ceil(res.msBeforeNext / 1000)) }
		});
	}

	// 4. BURST PROTECTION
	// Exception: If request is a conditional GET (cached), skip burst check
	// This accounts for "already checked" history navigations
	const isConditional =
		event.request.method === 'GET' &&
		(event.request.headers.has('if-none-match') || event.request.headers.has('if-modified-since'));

	if (!isConditional) {
		try {
			await burstLimiter.consume(ip);
		} catch (rej) {
			const res = rej as RateLimiterRes;
			return new Response(
				'Burst Limit Exceeded: Suspicious activity detected. You are blocked for 12 hours.',
				{
					status: 429,
					headers: { 'Retry-After': String(Math.ceil(res.msBeforeNext / 1000)) }
				}
			);
		}
	}

	return resolve(event);
};

// Exact shapes, not a prefix: a route added under /docs/ later must not inherit
// "no auth, no limit" by accident.
function isAgentDocs(pathname: string): boolean {
	return (
		pathname === '/llms.txt' ||
		pathname === '/llms-full.txt' ||
		pathname === '/docs' ||
		/^\/docs\/[^/]+$/.test(pathname)
	);
}
