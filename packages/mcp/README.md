<p align="center">
  <img src="https://raw.githubusercontent.com/Michael-Obele/tomoshibi/main/banner.svg" alt="Tomoshibi — lamp light banner with paper lantern" width="100%" />
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/v/tomoshi?label=tomoshi&color=cb0000" alt="npm version"/></a>
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/dm/tomoshi" alt="npm downloads"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi/actions/workflows/npm.yml"><img src="https://github.com/Michael-Obele/tomoshibi/actions/workflows/npm.yml/badge.svg" alt="npm provenance"/></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi"><img src="https://img.shields.io/github/stars/Michael-Obele/tomoshibi?style=social" alt="GitHub stars"/></a>
</p>

<h1 align="center">tomoshi <span style="font-weight:400">灯</span></h1>

<p align="center"><em>lamp light — the MCP that turns any site into LLM-ready markdown</em><br/>Self-hosted Firecrawl alternative. <strong>3 tools, not 17.</strong> One binary, $5/mo. Sigstore provenance.</p>

<p align="center">
  <a href="#install-30-seconds">Install</a> ·
  <a href="#run-locally-development">Run locally</a> ·
  <a href="#why-tomoshi">Why tomoshi</a> ·
  <a href="#tools">Tools</a> ·
  <a href="#project-structure">Structure</a> ·
  <a href="#hosting">Hosting</a>
</p>

---

## Why tomoshi?

If you're paying Firecrawl/Exa per token or spawning a browser per request, you're overpaying.

| What hurts with hosted APIs             | What tomoshi gives you                              | Outcome                             |
| --------------------------------------- | --------------------------------------------------- | ----------------------------------- |
| **$0.01–0.10 per scrape** + rate limits | **$0 self-hosted** — one binary, hobby-tier RAM     | Ship RAG without a cloud bill       |
| **500ms Chrome spawn per request**      | **One shared allocator + recycled tabs**            | ~200ms static, parallel pools       |
| **JS SPAs return empty HTML**           | **Smart mode** — static first, fallback to Chromedp | Works on React/Vue without guessing |
| **Noisy HTML (nav/ads/footer)**         | **Readability main-content** + ad block             | Clean markdown your LLM wants       |

Part of [**Tomoshibi 灯火**](https://github.com/Michael-Obele/tomoshibi) — Go API + Svelte 5 playground + this MCP. Formerly Cinder.

---

## Install (30 seconds)

**Prerequisites:** A running Tomoshibi API. Host it in one command:

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
docker compose up -d  # api 7431 + redis 7434 + searxng 7435
# or: fly deploy (see tomoshibi README)
```

Then add the MCP:

```jsonc
// .vscode/mcp.json or Claude config
{
  "mcpServers": {
    "tomoshi": {
      "command": "npx",
      "args": ["-y", "tomoshi"],
      "env": {
        "TOMOSHI_API_URL": "http://localhost:7431",
        // optional: forwarded to the backend as a search API key on every
        // request (X-Tomoshi-Env): set keys here, not in the backend's .env
        "BRAVE_SEARCH_API_KEY": "BSB-...",
      },
    },
  },
}
```

Legacy `CINDER_API_URL` still works. Aliases: `tomoshibi`, `tomoshibi-mcp` (same package).

Restart your editor — `tomoshi` appears as 3 tools.

---

## Run locally (development)

**Prerequisites:** [Bun](https://bun.sh) 1.2+ and a running Tomoshibi API (`http://localhost:7431`).

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi/packages/mcp
bun install
cp .env.example .env        # TOMOSHI_API_URL=http://localhost:7431

bun run dev                 # HTTP + SSE server → http://localhost:7433 (reloads on change)
bun run start               # same, without --watch
```

Two transports — pick one:

| Transport      | Command                                  | Use for                                |
| -------------- | ---------------------------------------- | -------------------------------------- |
| **stdio**      | `npx -y tomoshi` (bin → `dist/stdio.js`) | local editors: VS Code, Claude, Cursor |
| **HTTP + SSE** | `bun run dev` on `PORT` (7433)           | remote / hosted clients                |

HTTP endpoints: `POST` `/mcp` (Streamable HTTP), `/sse` + `/message` (legacy SSE), `/health` and `GET /` (JSON status).

Checks and build:

```bash
bun run typecheck   # tsc --noEmit
bun test            # unit tests
bun run check       # typecheck alias
bun run build       # dist/ — what the published bin runs
```

**Config** (see [.env.example](.env.example)): `TOMOSHI_API_URL` (required), `TOMOSHI_API_KEY`, `PORT`, `REDIS_URL` + `MCP_SESSION_MANAGER=redis` (sessions default to memory), `RATE_LIMIT_*`, `OAUTH_*`, `LOG_LEVEL`. Legacy `CINDER_API_URL` / `CINDER_API_KEY` still work.

**Search key forwarding:** names the backend asks for (`GET /v1/env`, e.g. `BRAVE_SEARCH_API_KEY`) are forwarded from this server's env on every request as `X-Tomoshi-Env`, so search API keys are configured once, here. `TOMOSHI_FORWARD_ENV` tunes it: `""` = all requested names (default), `"false"` = off, or a comma list to restrict. The backend re-validates against its own allowlist and resolves the keys per request; it never stores them.

---

## Docker

```bash
# from the repo root — starts on http://localhost:7433 (needs an API on 7431)
docker compose -f packages/mcp/docker-compose.yml up -d

curl http://localhost:7433/health
# → {"service":"tomoshi-mcp","status":"ok",...}
```

Point it somewhere else with `TOMOSHI_API_URL` (the name the server actually reads):

```bash
TOMOSHI_API_URL=https://your-api.example.com \
  docker compose -f packages/mcp/docker-compose.yml up -d
```

Published image:

```bash
docker run --rm -p 7433:7433 -e TOMOSHI_API_URL=http://host.docker.internal:7431 michaelobele/tomoshibi-mcp
```

---

## Tools

3 tools, not 8 — resource-oriented (`action` enum, ≤7 philosophy).

| Tool               | `action`                                                             | What it does                                                       |
| ------------------ | -------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `tomoshi_extract`  | `scrape` · `scrape_multi` (≤10) · `links` · `batch` · `batch_status` | Single/multi-page → markdown, link extraction, async batch (Redis) |
| `tomoshi_discover` | `search` · `map` · `crawl` · `crawl_status`                          | Web search (native roster → SearXNG → Brave/Serper/Tavily), sitemap, async BFS crawl |
| `tomoshi_monitor`  | `create` · `status` · `delete`                                       | Hash markdown, webhook on change                                   |

Async actions (`batch`, `crawl`, `monitor`) are Redis-backed — poll `*_status` until done.

**Example — scrape + search in one turn:**

> `tomoshi_discover` `search` "svelte 5 runes" → `tomoshi_extract` `scrape_multi` on top 3 URLs → LLM-ready markdown

---

## Hosting

| Option             | Command                                                     | Cost              |
| ------------------ | ----------------------------------------------------------- | ----------------- |
| **Docker Compose** | `docker compose -f packages/mcp/docker-compose.yml up -d`   | $0 (your machine) |
| **Fly.io**         | `fly deploy` (in `packages/mcp`)                            | ~$5/mo (512 MB)   |
| **From source**    | `bun run dev` (MCP) + `go run ./packages/api/cmd/api` (API) | $0 + Chromium     |

API base: `http://localhost:7431` — all endpoints under `/v1` (`/scrape`, `/search`, `/crawl`, `/batch`, `/monitor`, `/map`).

---

## Project structure

```text
packages/mcp/
├── src/
│   ├── stdio.ts            # stdio entry — what `npx tomoshi` runs (via dist/stdio.js)
│   ├── index.ts            # HTTP + SSE entry (srvx) — /mcp · /sse · /health on PORT
│   ├── server.ts           # McpServer wiring — registers the 3 tools
│   ├── client.ts           # typed HTTP client over the Go API (shared by the tools)
│   ├── config.ts           # Valibot env schema — TOMOSHI_API_URL, PORT, OAuth, limits
│   ├── adapter.ts          # valibot → JSON Schema adapter for tool inputs
│   ├── auth-provider.ts    # optional OAuth 2.1 (SimpleProvider)
│   └── tools/              # one file per tool, colocated bun tests
│       ├── extract.ts      # tomoshi_extract  — scrape · scrape_multi · links · batch · batch_status
│       ├── discover.ts     # tomoshi_discover — search · map · crawl · crawl_status
│       ├── monitor.ts      # tomoshi_monitor  — create · status · delete
│       ├── schema.ts       # shared field builders (num, url, …)
│       └── *.test.ts       # bun test suites
├── dist/                   # build output — published to npm, backs the bins
├── Dockerfile              # deps stage → oven/bun:1-slim (non-root, wget healthcheck)
├── docker-compose.yml      # mcp on 7433, needs an API on 7431
├── fly.toml                # Fly.io deploy
├── package.json            # bin: tomoshi / tomoshibi / tomoshibi-mcp → dist/stdio.js
├── tsconfig.json           # typecheck config (bun run check)
├── tsconfig.build.json     # build config → dist/
└── .env.example            # TOMOSHI_API_URL, PORT, sessions, rate limits, OAuth
```

---

## Links

- **GitHub:** https://github.com/Michael-Obele/tomoshibi
- **npm:** https://www.npmjs.com/package/tomoshi
- **API docs:** `https://your-api.fly.dev/swagger/index.html` (Swagger, auto-generated — debug mode only)

---

## Name

**tomoshi** (灯 — ともし) — Japanese for _lamp, light_. Short form of **Tomoshibi** (灯火 — ともしび) _lamp light_, the glow of a paper lantern. Search as illumination: lighting up the web for AI agents.

## License

MIT — see [LICENSE](https://github.com/Michael-Obele/tomoshibi/blob/main/LICENSE).
