import { error } from '@sveltejs/kit';
import { pages } from '$lib/docs-content';

export function load({ params }) {
const page = pages[params.slug];

if (!page) {
error(404, 'Page not found');
}

return {
page
};
}
