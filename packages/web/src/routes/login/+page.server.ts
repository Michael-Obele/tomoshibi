import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ cookies }) => {
	const password = env.MASTRA_PASSWORD;
	const authCookie = cookies.get('mastra_auth');

	// If already logged in with correct password, redirect home
	if (password && authCookie === password) {
		redirect(303, '/');
	}
	return {};
};
