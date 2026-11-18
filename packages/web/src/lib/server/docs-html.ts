import { rendered } from 'virtual:docs-html';

/** The build-time HTML for one document, or undefined when the slug is unknown. */
export function htmlFor(slug: string): string | undefined {
	return Object.hasOwn(rendered, slug) ? rendered[slug] : undefined;
}
