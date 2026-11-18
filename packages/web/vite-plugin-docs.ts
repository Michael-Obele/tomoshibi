import { createHash } from 'node:crypto';
import { existsSync, readFileSync, realpathSync } from 'node:fs';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Marked } from 'marked';
import { createHighlighter, type Highlighter } from 'shiki';
import type { Plugin } from 'vite';

const docsId = 'virtual:docs';
const htmlId = 'virtual:docs-html';

// Relative links in the markdown point at repo files, which have no URL on the
// site; they resolve to the source (or to the published page when one exists).
const repoBlob = 'https://github.com/Michael-Obele/tomoshibi/blob/main';

const languages = [
	'bash',
	'diff',
	'dockerfile',
	'go',
	'html',
	'javascript',
	'json',
	'jsonc',
	'markdown',
	'python',
	'svelte',
	'text',
	'toml',
	'typescript',
	'yaml'
];

type Source = {
	slug: string;
	title: string;
	summary: string;
	section: string;
	path: string;
};

type Doc = {
	slug: string;
	title: string;
	summary: string;
	section: string;
	url: string;
	source: string;
	bytes: number;
	sha256: string;
};

type Manifest = {
	version: string;
	api_version: string;
	doc_count: number;
	docs: Doc[];
};

type Section = { title: string; docs: { slug: string; title: string; summary: string }[] };

/** Repo root, found by walking up from this file so any cwd works. */
function findRepo(): string {
	let dir = dirname(fileURLToPath(import.meta.url));
	for (let i = 0; i < 5; i++) {
		if (existsSync(join(dir, 'docs.sources.json'))) return dir;
		dir = dirname(dir);
	}
	if (existsSync(join(process.cwd(), 'docs.sources.json'))) return process.cwd();
	throw new Error(`docs: docs.sources.json not found above ${fileURLToPath(import.meta.url)}`);
}

const sha = (data: string | Buffer) => createHash('sha256').update(data).digest('hex');

const escapeHtml = (text: string) =>
	text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

function apiVersion(repo: string): string {
	try {
		const raw = readFileSync(join(repo, 'packages/api/internal/version/version.go'), 'utf8');
		return /const Version = "([0-9A-Za-z._+-]+)"/.exec(raw)?.[1] ?? 'unknown';
	} catch {
		return 'unknown';
	}
}

/** Renders the catalog exactly as the docs routes describe it. */
function build(catalog: { intro: string; sources: Source[] }, repo: string) {
	if (!catalog.intro?.startsWith('# ')) {
		throw new Error('docs: docs.sources.json must open with an H1');
	}
	if (!catalog.sources?.length) {
		throw new Error('docs: docs.sources.json lists no documents');
	}

	const realRepo = realpathSync(repo);
	// Null-prototype: a slug like `__proto__` would otherwise set the prototype.
	const documents: Record<string, string> = Object.create(null);
	const docs: Doc[] = [];
	const sections = new Map<string, Doc[]>();

	for (const source of catalog.sources) {
		if (!source.slug || /[\\/]/.test(source.slug)) {
			throw new Error(`docs: slug ${JSON.stringify(source.slug)} is not a plain file name`);
		}
		if (documents[source.slug] !== undefined) {
			throw new Error(`docs: duplicate slug ${source.slug}`);
		}

		// The catalog is repo data, but a bad entry must not read outside the repo —
		// realpath, because a committed symlink would pass a lexical check.
		const file = realpathSync(resolve(repo, source.path));
		const rel = relative(realRepo, file);
		if (rel.startsWith('..') || isAbsolute(rel)) {
			throw new Error(`docs: ${source.path} escapes the repo`);
		}

		const body = readFileSync(file, 'utf8');
		if (!body.trim()) throw new Error(`docs: ${source.path} is empty`);

		const doc: Doc = {
			slug: source.slug,
			title: source.title,
			summary: source.summary,
			section: source.section,
			url: `/docs/${source.slug}.md`,
			source: source.path,
			bytes: Buffer.byteLength(body),
			sha256: sha(body)
		};
		documents[source.slug] = body;
		docs.push(doc);
		sections.set(source.section, [...(sections.get(source.section) ?? []), doc]);
	}

	// Same digest shape the manifest always had: the identity of every document,
	// not its position in the list.
	const version = sha(docs.map((d) => `${d.slug}\0${d.sha256}\0`).join('')).slice(0, 12);
	const manifest: Manifest = {
		version,
		api_version: apiVersion(repo),
		doc_count: docs.length,
		docs
	};

	let index = catalog.intro;
	for (const [section, list] of sections) {
		index += `\n\n## ${section}\n\n`;
		for (const doc of list) index += `- [${doc.title}](${doc.url}): ${doc.summary}\n`;
	}
	index += `\n\n## About this file\n\n`;
	index += `- Docs version \`${version}\`, describing API \`${manifest.api_version}\`, ${docs.length} documents.\n`;
	index += `- Every link above is served by this same origin. \`/docs\` is the machine-readable manifest (version, per-document sha256, sizes).\n`;
	index += `- \`/llms-full.txt\` is every document concatenated, for a large context window.\n`;
	index += `- \`/docs\` returns the same manifest as JSON; compare its \`version\` to know whether what you cached is current.\n`;
	index += `- Paths are origin-relative: prefix them with the base URL you are calling.\n`;

	let full = catalog.intro + '\n\n---\n';
	for (const doc of docs) full += `\n\n## ${doc.title}\n\n${documents[doc.slug]}\n\n---\n`;

	const sectionList: Section[] = [...sections].map(([title, list]) => ({
		title,
		docs: list.map(({ slug, title: docTitle, summary }) => ({
			slug,
			title: docTitle,
			summary
		}))
	}));

	return { manifest, llmsIndex: index, llmsFull: full, documents, sections: sectionList };
}

const themes = { light: 'github-light', dark: 'github-dark' } as const;

let highlighter: Promise<Highlighter> | undefined;

function getHighlighter(): Promise<Highlighter> {
	highlighter ??= createHighlighter({ themes: [themes.light, themes.dark], langs: languages });
	return highlighter;
}

// The copy/check icons are lucide-svelte components rendered by the page and
// cloned in at mount, so this markup leaves the button's icon slot empty.

/**
 * Every code block is this shell: a bar with the fence language and a copy
 * button. The page adds one delegated click handler and the icons, the markup
 * stays static.
 */
function codeBlock(html: string, lang: string): string {
	const label = /^[a-z][\w+#.-]*$/.test(lang) ? lang : 'text';
	return (
		`<div class="code-block" data-lang="${label}">` +
		`<div class="code-block__bar"><span class="code-block__lang">${label}</span>` +
		`<button type="button" class="code-block__copy" data-copy aria-label="Copy code">` +
		`<span class="code-block__label" aria-live="polite">Copy</span>` +
		`</button></div>${html}</div>`
	);
}

async function highlight(code: string, lang: string): Promise<string> {
	if (lang) {
		try {
			const shiki = await getHighlighter();
			// Dual themes: tokens carry both colours, layout.css picks one.
			return codeBlock(shiki.codeToHtml(code, { lang, themes, defaultColor: false }), lang);
		} catch {
			// Unknown fence language falls through to the plain block below.
		}
	}
	return codeBlock(
		`<pre class="shiki" tabindex="0"><code>${escapeHtml(code)}</code></pre>`,
		lang
	);
}

const token = (index: number) => `<!--tomoshibi-code:${index}-->`;

/**
 * Markdown -> HTML once, at build time. Code blocks become tokens during the
 * parse and are swapped for Shiki output afterwards, so the renderer stays
 * synchronous while the highlighter does not.
 */
async function renderAll(
	repo: string,
	docs: Doc[],
	documents: Record<string, string>
): Promise<Record<string, string>> {
	const slugBySource = new Map(docs.map((doc) => [doc.source, doc.slug]));
	const blocks: { code: string; lang: string }[] = [];
	const marked = new Marked({ gfm: true });
	marked.use({
		renderer: {
			code({ text, lang }) {
				blocks.push({ code: text, lang: (lang ?? '').trim().toLowerCase() });
				return token(blocks.length - 1);
			}
		}
	});

	const rendered: Record<string, string> = Object.create(null);
	for (const doc of docs) {
		const markdown = relink(documents[doc.slug], repo, doc.source, slugBySource);
		if (markdown.includes('<!--tomoshibi-code:')) {
			throw new Error(`docs: ${doc.source} contains the renderer's own placeholder`);
		}

		blocks.length = 0;
		// The page renders the catalog title, so drop the document's own H1 —
		// wherever it sits, because three READMEs open with a badge block.
		let html = (marked.parse(markdown) as string).replace(/<h1[^>]*>[\s\S]*?<\/h1>/, '');
		for (let i = 0; i < blocks.length; i++) {
			const code = await highlight(blocks[i].code, blocks[i].lang);
			// A function, not a string: `$&` and friends in the HTML would otherwise
			// be treated as replacement patterns.
			html = html.replace(token(i), () => code);
		}
		if (html.includes('<!--tomoshibi-code:')) {
			throw new Error(`docs: ${doc.source} left a code placeholder unrendered`);
		}
		rendered[doc.slug] = html;
	}
	return rendered;
}

/** Repoints relative links at the published page, or at the file on GitHub. */
function relink(
	markdown: string,
	repo: string,
	source: string,
	slugBySource: Map<string, string>
): string {
	const base = dirname(join(repo, source));
	return markdown.replace(/\]\((?!https?:|#|mailto:)([^)\s]+)\)/g, (whole, target: string) => {
		const [path, hash] = target.split('#');
		if (!path) return whole;

		// Links are written relative to their own document, not to the repo root.
		const absolute = resolve(base, path);
		const rel = relative(repo, absolute);
		if (rel.startsWith('..') || isAbsolute(rel) || !existsSync(absolute)) return whole;

		const slug = slugBySource.get(rel);
		const href = slug ? `/docs/${slug}` : `${repoBlob}/${rel}`;
		return `](${href}${hash ? `#${hash}` : ''})`;
	});
}

/** Exposes the docs as `virtual:docs` + `virtual:docs-html`, rebuilt on every request. */
export function docs(): Plugin {
	const resolveId = (id: string) =>
		id === docsId ? '\0' + docsId : id === htmlId ? '\0' + htmlId : null;

	return {
		name: 'tomoshibi-docs',
		resolveId,
		async load(id) {
			if (id !== '\0' + docsId && id !== '\0' + htmlId) return null;

			const repo = findRepo();
			const catalog = JSON.parse(readFileSync(join(repo, 'docs.sources.json'), 'utf8'));
			const { manifest, llmsIndex, llmsFull, documents, sections } = build(catalog, repo);

			if (id === '\0' + htmlId) {
				const rendered = await renderAll(repo, manifest.docs, documents);
				return `export const rendered = ${JSON.stringify(rendered)};`;
			}

			return [
				`export const manifest = ${JSON.stringify(manifest)};`,
				`export const llmsIndex = ${JSON.stringify(llmsIndex)};`,
				`export const llmsFull = ${JSON.stringify(llmsFull)};`,
				`export const documents = ${JSON.stringify(documents)};`,
				`export const sections = ${JSON.stringify(sections)};`
			].join('\n');
		},
		configureServer(server) {
			const repo = findRepo();
			const catalog = JSON.parse(readFileSync(join(repo, 'docs.sources.json'), 'utf8'));
			const watched = new Set([join(repo, 'docs.sources.json')]);
			for (const source of catalog.sources as Source[]) watched.add(join(repo, source.path));

			server.watcher.add([...watched]);
			server.watcher.on('change', (file) => {
				if (!watched.has(file)) return;
				for (const virtualId of [docsId, htmlId]) {
					const module = server.moduleGraph.getModuleById('\0' + virtualId);
					if (module) server.moduleGraph.invalidateModule(module);
				}
				server.ws.send({ type: 'full-reload' });
			});
		}
	};
}
