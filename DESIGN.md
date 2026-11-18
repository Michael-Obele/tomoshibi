# Design notes

The long-form companion to the code. Code comments stay short — one to three lines
saying *why*, next to the lines they explain. Anything that needs a paragraph lives
here instead, and the comment points at the section by name.

This is a reading document, not a rulebook. Rules that a reviewer must enforce live
in `AGENTS.md`; decisions that must outlive a refactor live in `docs/adr/`.

## Docs pipeline

### One host, generated at build

`docs.sources.json` at the repo root is the whole published surface: which
documents, in what order, under which H2, with the note an agent reads before
following a link, plus the preamble that opens the index. `packages/web` renders it
during every build and every dev request, so the site is the only host — the Go API
serves `/v1` and its probes.

There is no second copy anywhere, which is what makes "the docs cannot disagree with
the product" true by construction rather than checked after the fact. The Go API used
to embed its own render; it was removed on 2026-11-17 because nobody exposes a local
API to an agent, and a second copy bought a parity check rather than a reader.

### Rendered markdown is trusted repo content

The doc pages inject build-time HTML with `{@html}`. The input is this repository's
own markdown — including package READMEs, which a pull request can edit — so a
merged commit could put script on a docs page. That is the trust boundary: merging
to `main` is the permission, and the same commit can change the app itself.

The blast radius is bounded rather than the content sanitised: `svelte.config.js`
sets a nonce-based CSP, so inline `<script>` and `onerror=` handlers in markdown are
blocked while badges and tables still render. A sanitiser would have to be pinned
and kept current; a policy that denies script needs neither.

### Why generated at all

"Docs that stay up to date" is not a promise you can keep with a second copy. The
tracked markdown is the source of truth; the served copy is produced at build time,
so there is no artifact to forget, no `-check` to run, and no way to deploy
yesterday's text. Editing a README shows up on the next reload in dev.

### Why a Vite plugin, and a virtual module

`vite-plugin-docs.ts` reads the catalog and the markdown and exposes `virtual:docs`
(`manifest`, `llmsIndex`, `llmsFull`, `documents`). Three alternatives, and why not:

- **A committed `docs.generated.ts`.** Needs its own `-check` step and can still be
  stale between the edit and the check. The site is the one host with no excuse for a
  stale copy, so it should not be possible to have one.
- **`import.meta.glob` over `content/*.md` with `?raw`.** Needs bundler query syntax, a
  second pattern that must stay in step with the manifest, and `vite/client` types this
  project does not load.
- **A script run from `prebuild`.** Works, but it is a second entry point (`predev` too)
  for something Vite already does on every build and every dev request.

The plugin also watches the catalog and every source file, so editing a README
invalidates the module and reloads in dev. The module is ~330 KB and server-only: no
browser bundle imports it.

### Why `+server.ts` in a folder with a dot in its name

SvelteKit maps a route directory to a URL literally, dots included, so
`src/routes/llms.txt/+server.ts` is `/llms.txt` and no rewriting rule is needed:

| Route directory | URL | Body |
| --- | --- | --- |
| `routes/llms.txt/+server.ts` | `/llms.txt` | the index |
| `routes/llms-full.txt/+server.ts` | `/llms-full.txt` | every document concatenated |
| `routes/docs/+server.ts` | `/docs` | the manifest, as JSON |
| `routes/docs/[file=md]/+server.ts` | `/docs/<slug>.md` | one document |

`/docs` and `/docs/<slug>` also have human pages. SvelteKit allows a `+page` and a
`+server` in the same directory: `PUT`/`PATCH`/`DELETE` always go to the server, and
`GET` is a page request only when `Accept` prioritises `text/html`. So the same URL
serves the site to a browser and markdown to an agent without two URL schemes drifting
apart.

Measured, not assumed: the page response did **not** carry `Vary: Accept`, so
`hooks.server.ts` sets it for `/docs`. Without it a shared cache could hand the JSON
manifest to a browser.

The `[file=md]` matcher (`src/params/md.ts`) is what keeps `/docs/api-reference.md`
out of the human `/docs/[slug]` page, whose slots are the catalog slugs. Parameters
with matchers outrank parameters without, and a failed match falls through to the
next route.

### Why these routes are not prerendered

Prerendering would make them static files, and a static file cannot answer
`If-None-Match` or stamp `X-Docs-Version` — the two things that let an agent tell
whether the docs it cached are current. Ten small dynamic responses are cheaper than
losing the revalidation contract, and the routes send the same
version/ETag/revalidation/`Link` headers on every host instead of one host's
`_headers` file.

### Why relative links

Every link the generator writes is origin-relative (`/docs/api-reference.md`, not
`https://example.com/...`). The same bytes are then correct on a laptop at
`localhost:7431`, on a self-hosted instance, and on the public site — and "what is
the production domain" stops being a blocker for publishing. An agent that has the
file already knows the origin it fetched it from.

### What `llms.txt` v2 actually asks for

Read from <https://llmstxt.org/> (v2, August 2026), because the v1 habits are still
widespread and some of them are now wrong:

- Structure, in order: optional BOM, **H1** (the only required part), a blockquote
  summary, any markdown except headings, then H2 sections holding file lists. Each
  item is `[name](url)` optionally followed by `: notes`.
- A file may live at any path and covers the pages under it; when two apply, the
  more specific one wins.
- Markdown versions of pages are proposed at `page.html.md` or `page.md`, advertised
  with `rel="alternate" type="text/markdown"`; the covering index is advertised with
  `rel="describedby"`. Either can be HTML `<link>` elements or an HTTP `Link:` header.
- `llms-full.txt` is **not** part of the spec. It is a Mintlify convention that other
  sites copied. It is published because a large context window is a legitimate way to
  read this corpus, and its shape is documented on the index page itself.
- `## Optional` is a convention for secondary links. Since v2 dropped the
  `llms_txt2ctx` tooling, it carries no mechanical meaning — no tool omits it for you.

Corpus, measured 2026-11-16: 10 documents, 159 kB, ≈ 39.8k tokens, with effectively
no cross-document duplication. It is large because the product is.

### Deliberately not done

- **`/.well-known/llms.txt`.** Mintlify publishes one. Under v2's subpath rule it
  would claim to cover `/.well-known/`, which is not what it means. One index, one
  location.
- **Demoting headings inside `llms-full.txt`.** Nice for page boundaries, but the
  rewrite has to survive code fences, and a wrong demotion silently corrupts a
  document. The per-document files are the reliable boundary.
- **Compressing the responses ourselves.** `/llms-full.txt` is ~161 KB. Netlify and
  Docker deployments both put a CDN or reverse proxy in front; gzip belongs there,
  not in a route handler that would then have to negotiate `Accept-Encoding`.
