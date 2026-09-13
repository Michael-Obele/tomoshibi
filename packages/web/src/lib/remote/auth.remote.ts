import { form, query } from '$app/server';
import { env } from '$env/dynamic/private';
import { dev } from '$app/environment';
import { getRequestEvent } from '$app/server';
import { redirect } from '@sveltejs/kit';
import * as v from 'valibot';

export const loginUser = form(
	v.object({
		password: v.pipe(v.string(), v.nonEmpty('Password is required to elevate.'))
	}),
	async ({ password }) => {
		const configuredPassword = env.MASTRA_PASSWORD || 'cinder'; // Fallback for demo

		const event = getRequestEvent();
		if (!event) throw new Error('Request event not found');

		// Fake delay
		await new Promise((resolve) => setTimeout(resolve, 800));

		if (password !== configuredPassword) {
			throw new Error('Incorrect credentials. You remain among the others.');
		}

		event.cookies.set('mastra_auth', password, {
			path: '/',
			httpOnly: true,
			secure: !dev,
			maxAge: 60 * 60 * 24 * 30, // 30 days
			sameSite: 'lax'
		});

		return { success: true, user: 'Master' };
	}
);

export const logoutUser = form(v.object({}), async () => {
	const event = getRequestEvent();
	if (event) {
		event.cookies.delete('mastra_auth', { path: '/' });
	}
	return { success: true };
});

export const getAuthStatus = query(async () => {
	const event = getRequestEvent();
	if (!event) return { user: null };

	const authCookie = event.cookies.get('mastra_auth');
	const configuredPassword = env.MASTRA_PASSWORD || 'cinder';

	if (authCookie && authCookie === configuredPassword) {
		return { user: { name: 'Master' } };
	}

	return { user: null };
});
