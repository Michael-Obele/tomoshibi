# Contributing to Tomoshibi

Everything a contributor needs that isn't already in a package README. The
[main README](./README.md) is the front door; each package's README owns its own
detail.

## Repository layout

```text
tomoshibi/
├── packages/
│   ├── api/            # Go API — Gin + Colly + Chromedp + embedded Asynq worker (7431)
│   ├── mcp/            # MCP server — 3 tools over the API, npm `tomoshi` (7433)
│   ├── web/            # Svelte 5 playground (7432)
│   └── bridge/         # Browser extension — WXT + Svelte 5 + TypeScript (PolyForm NC)
├── docs/               # tracked: SEARCH_COMPARISON, API_REFERENCE, SEARXNG_FLY, adr/
├── skills/             # agent skills (indexed by skills.sh; mirrored to .agents/, .agent/)
├── plan/               # local design plans and release notes (gitignored)
├── .github/workflows/  # ci.yml, docker.yml (images), npm.yml (publish)
├── docker-compose.yml  # full stack: api 7431 + redis 7434 (+ searxng 7435 opt-in)
├── AGENTS.md           # agent context source of truth (CLAUDE.md symlinks to it)
└── package.json        # bun workspaces — dev:api · dev:mcp · dev:web · check
```

## Prerequisites

| Need | For |
| --- | --- |
| Go 1.25+ | `packages/api` |
| Bun 1.4+ | `packages/{mcp,web,bridge}` and the scripts |
| Chromium or Chrome for Testing | `dynamic` scrapes; the extension E2E suite |
| Redis (optional) | crawl, batch, monitor |

## Running the stack

```bash
# full stack
docker compose up -d
# per package
docker compose -f packages/api/docker-compose.yml up -d    # api + redis (searxng: --profile searxng)
docker compose -f packages/mcp/docker-compose.yml up -d    # mcp (needs api)
docker compose -f packages/web/docker-compose.yml up -d    # web (needs api)
```

Published images, on both registries:

```bash
docker pull michaelobele/tomoshibi-api        # api only (7431)
docker pull michaelobele/tomoshibi-mcp        # mcp only (7433)
docker pull michaelobele/tomoshibi-web        # web only (7432)
docker pull michaelobele/tomoshibi            # all-in-one
# same images: ghcr.io/michael-obele/tomoshibi-*
```

**The web image bakes its API URL at build time** (`http://api:7431`), so run it
through its compose file — that overrides the build arg. A bare
`docker run … tomoshibi-web` will point at the wrong host.

## Local development

```bash
go run ./packages/api/cmd/api                 # api (7431)
bun --cwd packages/mcp dev                    # mcp (7433)
bun --cwd packages/web dev                    # web (5173)
bun --cwd packages/bridge dev                 # extension in a dev Chrome, HMR
```

## Checks before a PR

| Package | Command | What it covers |
| --- | --- | --- |
| api | `make check` | gofmt, `go vet`, `staticcheck`, tests |
| bridge | `bun run check` | `wxt prepare`, `tsc --noEmit`, unit tests |
| bridge (E2E) | `bun test/e2e-protocol.ts` · `bun test/e2e-extension.ts` | protocol against real binaries; the extension in real Chrome |
| mcp | `bun run check && bun test` | typecheck, 60+ tests |
| web | `bun run check` | svelte-check |

CI runs the same commands, plus `-race` for Go and both E2E suites. A failing
check is never fixed by relaxing it.

## Updating Docker

| You changed | Do this locally | CI does on push to `main` |
| --- | --- | --- |
| Go code (`packages/api`) | `docker compose up -d --build api` | builds and pushes `ghcr.io/michael-obele/tomoshibi-*` (+ Docker Hub) |
| MCP (`packages/mcp`) | `docker compose -f packages/mcp/docker-compose.yml up -d --build` | npm workflow publishes `tomoshi` |
| Web (`packages/web`) | `docker compose -f packages/web/docker-compose.yml up -d --build` | same Docker workflow |
| compose / `.env` only | `docker compose up -d` (no `--build`) | — |

## Release checklist

1. Bump the versions that changed: root `package.json`, `packages/api`
   (`GET /` advertises it), `packages/{bridge,mcp,web}/package.json`.
2. `make check` in `packages/api`; `bun run check` in `packages/bridge` and
   `packages/mcp`.
3. Branch → PR → wait for CI → merge → tag `v<version>` → push the tag.
4. `gh run list` — watch the Docker and npm workflows.
5. Verify: `docker logs tomoshibi-api-1 2>&1 | grep -E "Native search engines loaded|Telemetry recording"` and `curl -s localhost:7431/health`.

Self-hosters pull the same images CI pushed:
`docker pull ghcr.io/michael-obele/tomoshibi-api:latest`.

## Where the local-only docs live

`plan/` and most of `docs/` are gitignored on purpose — design plans, release
notes and research logs stay on disk but out of the public repo. Only
`docs/SEARCH_COMPARISON.md`, `docs/guides/API_REFERENCE.md` and
`docs/guides/SEARXNG_FLY.md` are tracked.
