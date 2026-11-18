declare module 'virtual:docs' {
	export type Doc = {
		slug: string;
		title: string;
		summary: string;
		section: string;
		url: string;
		source: string;
		bytes: number;
		sha256: string;
	};

	export type Manifest = {
		version: string;
		api_version: string;
		doc_count: number;
		docs: Doc[];
	};

	export type Section = {
		title: string;
		docs: { slug: string; title: string; summary: string }[];
	};

	export const manifest: Manifest;
	export const llmsIndex: string;
	export const llmsFull: string;
	export const documents: Record<string, string>;
	export const sections: Section[];
}

declare module 'virtual:docs-html' {
	export const rendered: Record<string, string>;
}
