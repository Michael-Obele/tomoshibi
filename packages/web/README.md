<p align="center">
  <a href="https://svelte.dev"><img src="https://img.shields.io/badge/Svelte-5-ff3e00?logo=svelte&logoColor=white" alt="Svelte 5"/></a>
  <a href="https://kit.svelte.dev"><img src="https://img.shields.io/badge/SvelteKit-2-white?logo=svelte&logoColor=black" alt="SvelteKit"/></a>
  <a href="https://tailwindcss.com"><img src="https://img.shields.io/badge/Tailwind-4-38bdf8?logo=tailwindcss&logoColor=white" alt="Tailwind 4"/></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi"><img src="https://img.shields.io/github/stars/Michael-Obele/tomoshibi?style=social" alt="GitHub stars"/></a>
</p>

# Tomoshibi Web _(packages/web)_

Svelte 5 playground for Tomoshibi — a browser UI for scrape, crawl and search.

The browser playground for [**Tomoshibi 灯火**](../../README.md) — a SvelteKit UI that drives the scraping API: try a scrape, run a crawl, fire a search, read the docs, all from a local page.

- **Routes** — `/` homepage · `/playground` scrape/crawl/search UI · `/docs` API docs · `/login` password gate
- **Server-side proxy** — remote functions call the Go API from the server, so API keys never reach the browser
- **Rate limited** — global + per-IP + burst protection on every unauthenticated request
- **UI** — Tailwind CSS v4, shadcn-svelte-style components (Bits UI), lucide icons, dark/light mode

> Requires a running Tomoshibi API. See [`packages/api/README.md`](../api/README.md).

**Contents:** [Run locally](#run-locally) · [Run with Docker](#run-with-docker) · [Project structure](#project-structure) · [Troubleshooting](#troubleshooting) · [Related](#related)

---

## Run locally

**Prerequisites:** [Bun](https://bun.sh) 1.2+ and the API on `http://localhost:7431`.

```bash
cd packages/web
bun install
cp .env.example .env          # required — see Config below
bun run dev                   # → http://localhost:5173
```

Start the API first (from `packages/api`: `go run ./cmd/api` or `docker compose up -d`).

### Config

`.env` in this directory (SvelteKit loads it automatically):

| Variable                      | Required | When read                                   | Purpose                                                            |
| ----------------------------- | -------- | ------------------------------------------- | ------------------------------------------------------------------ |
| `PRIVATE_TOMOSHI_BACKEND_URL` | **yes**  | **build/dev start** (`$env/static/private`) | API base URL, e.g. `http://localhost:7431`                         |
| `PRIVATE_TOMOSHI_API_KEY`     | no       | runtime                                     | API key, only if the API has `APP_API_KEYS` set                    |
| `MASTRA_PASSWORD`             | no       | runtime                                     | password for `/login`; the resulting cookie also skips rate limits |

> `PRIVATE_TOMOSHI_BACKEND_URL` is baked in when Vite starts — **restart `bun run dev` after changing it**, or the app throws `PRIVATE_TOMOSHI_BACKEND_URL is not set`. The other two are read per request.

### Scripts

| Command                           | What it does                               |
| --------------------------------- | ------------------------------------------ |
| `bun run dev`                     | dev server on `5173`                       |
| `bun run build`                   | production build                           |
| `bun run preview`                 | serve the production build                 |
| `bun run check`                   | `svelte-kit sync` + `svelte-check` (types) |
| `bun run lint` / `bun run format` | Prettier check / write                     |

---

## Run with Docker

```bash
# from the repo root — serves on http://localhost:7432
docker compose -f packages/web/docker-compose.yml up -d

# point it at an API that isn't on this machine's localhost
PRIVATE_TOMOSHI_BACKEND_URL=http://192.168.1.10:7431 \
  docker compose -f packages/web/docker-compose.yml up -d --build
```

Details worth knowing:

- The backend URL is a **build argument** (`$env/static/private`), so the container URL is fixed at image build time — re-run `--build` when it changes. The compose file defaults to `http://host.docker.internal:7431` and maps `host.docker.internal` to the host gateway, so it reaches an API running outside Docker (e.g. the root `docker compose up -d` stack, or `go run ./cmd/api`).
- `MASTRA_PASSWORD` is a **runtime** env var — pass it with `-e MASTRA_PASSWORD=...` or add it to a `.env` next to the compose file.
- Image is two-stage: `oven/bun:1` builds (`DOCKER_BUILD=1` → `adapter-node`), `oven/bun:1-slim` serves `build/index.js` on port `7432`.

### With the rest of the stack

```bash
# 1. API + Redis + SearXNG
docker compose -f packages/api/docker-compose.yml up -d
# 2. this UI
docker compose -f packages/web/docker-compose.yml up -d
```

---

## Project structure

```text
packages/web/
├── src/
│   ├── routes/                 # SvelteKit file-based routing
│   │   ├── +page.svelte        # / — homepage: pitch, features, curl examples
│   │   ├── playground/         # /playground — interactive scrape / crawl / search UI
│   │   ├── docs/               # /docs — API documentation pages
│   │   ├── login/              # /login — password gate (MASTRA_PASSWORD)
│   │   ├── +layout.svelte      # app shell: nav, theme, toasts
│   │   └── layout.css          # Tailwind v4 theme tokens
│   ├── lib/
│   │   ├── remote/             # SvelteKit remote functions → server-side API proxy
│   │   ├── server/             # rate-limiter (global / IP / burst) + server helpers
│   │   ├── components/         # shadcn-svelte-style UI — ui/ · blocks/ · brand/
│   │   ├── hooks/              # reusable runes (is-mobile.svelte.ts)
│   │   ├── assets/             # images used by components
│   │   ├── docs-content.ts     # copy for the /docs pages
│   │   ├── index.ts            # barrel exports
│   │   └── utils.ts            # cn() — clsx + tailwind-merge
│   ├── hooks.server.ts         # auth cookie check + 3-tier rate limiting
│   ├── app.html / app.d.ts     # HTML shell + global types
├── static/                     # favicon.svg, robots.txt (served at /)
├── svelte.config.js            # adapter-auto (local) / adapter-node (DOCKER_BUILD=1)
├── vite.config.ts              # Vite + Tailwind + SvelteKit plugins
├── Dockerfile                  # bun build → bun:1-slim runtime, port 7432
├── docker-compose.yml          # web on 7432; backend URL passed as a build arg
├── package.json                # dev · build · preview · check · lint · format
└── .env.example                # PRIVATE_TOMOSHI_* + MASTRA_PASSWORD
```

---

## Troubleshooting

| Symptom                                  | Fix                                                                                                                                          |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `PRIVATE_TOMOSHI_BACKEND_URL is not set` | `cp .env.example .env` and set it, then restart the dev server                                                                               |
| Requests fail with connection refused    | the API isn't running — start it on `7431`, or point the URL at where it is                                                                  |
| `401` from the API                       | API auth is on — set `PRIVATE_TOMOSHI_API_KEY` in `.env`                                                                                     |
| Login always says incorrect credentials  | set `MASTRA_PASSWORD` in `.env`; it is compared strictly, with no default                                                                    |
| `429` / `503` while browsing             | rate limits hit (unauthenticated traffic is limited) — log in with `MASTRA_PASSWORD` or raise the limits in `src/lib/server/rate-limiter.ts` |
| Docker UI can't reach the API            | the URL is a build arg — `--build` after changing it; for a host API make sure `extra_hosts: host-gateway` is present (compose default)      |
| Port 5173 busy                           | `bun run dev -- --port 5174`                                                                                                                 |

---

## Related

- [Repo README](../../README.md) — project overview and full-stack quick start
- [API package](../api/README.md) — the backend this UI talks to
- [MCP package](../mcp/README.md) — same API, exposed as 3 MCP tools

## License

MIT — see [LICENSE](../../LICENSE).
