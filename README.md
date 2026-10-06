<p align="center">
  <img src="./banner.svg" alt="Tomoshibi — lamp light banner with paper lantern and Japanese characters" width="100%" />
</p>

<p align="center">
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License"/></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25+"/></a>
  <a href="https://bun.sh"><img src="https://img.shields.io/badge/Bun-1.2%2B-f9f1e1?logo=bun" alt="Bun"/></a>
  <a href="https://svelte.dev"><img src="https://img.shields.io/badge/Svelte-5-ff3e00?logo=svelte&logoColor=white" alt="Svelte 5"/></a>
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/v/tomoshi?label=tomoshi&color=cb0000" alt="npm version"/></a>
  <a href="https://www.npmjs.com/package/tomoshi"><img src="https://img.shields.io/npm/dm/tomoshi" alt="npm downloads"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi/actions/workflows/npm.yml"><img src="https://github.com/Michael-Obele/tomoshibi/actions/workflows/npm.yml/badge.svg" alt="npm provenance"/></a>
  <a href="https://github.com/Michael-Obele/tomoshibi"><img src="https://img.shields.io/github/stars/Michael-Obele/tomoshibi?style=social" alt="GitHub stars"/></a>
</p>

<h1 align="center">Tomoshibi <span style="font-weight:400">灯火</span></h1>

<p align="center"><em>lamp light — self-hosted search &amp; scraping for AI agents</em><br/>Turn any site into LLM-ready markdown in 200ms. One binary, $5/mo. Formerly <a href="https://github.com/Michael-Obele/cinder">Cinder</a> — <code>tomoshi</code> CLI alias.</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#packages">Packages</a> ·
  <a href="#api">API</a> ·
  <a href="#mcp">MCP</a> ·
  <a href="#web">Web</a> ·
  <a href="#benchmarks">Benchmarks</a>
</p>

---

## Why Tomoshibi

If you're paying Firecrawl/Exa by the token or spawning a Playwright per request, you're overpaying.

| What hurts with hosted APIs             | What Tomoshibi gives you                            | Outcome                             |
| --------------------------------------- | --------------------------------------------------- | ----------------------------------- |
| **$0.01–0.10 per scrape** + rate limits | **$0 self-hosted** — one binary, hobby-tier RAM     | Ship RAG without a cloud bill       |
| **500ms Chrome spawn per request**      | **One shared allocator + recycled tabs**            | ~200ms static, parallel pools       |
| **JS SPAs return empty HTML**           | **Smart mode** — static first, fallback to Chromedp | Works on React/Vue without guessing |
| **Noisy HTML (nav/ads/footer)**         | **Readability main-content** + ad block             | Clean markdown your LLM wants       |
| **Crawl needs a worker fleet**          | **Monolith** — Gin + Asynq in one process           | Pay per container, not per service  |

**Proof:** SearXNG search benchmark (`scripts/search-bench.py`, 10 workers/30s) — **Tomoshibi 560 req/s p50 11ms** vs Firecrawl self-hosted **1.9 req/s p50 5.4s** — ~300× throughput, same $0. See [`docs/SEARCH_COMPARISON.md`](docs/SEARCH_COMPARISON.md).

---

## Quick start

### Full stack — Docker Compose (recommended)

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
docker compose up -d              # api (7431) + redis (7434) + searxng (7435)
curl http://localhost:7431/health   # → {"status":"ok","service":"tomoshibi"}

# optional front doors to the same API:
docker compose -f packages/web/docker-compose.yml up -d --build   # web UI (7432)
docker compose -f packages/mcp/docker-compose.yml up -d           # MCP   (7433)
```

Ports `7431`–`7435` (avoids clashing with 3000/8000/8080). Every feature works out of the box — scrape, crawl, batch, monitor, search.

### Individual services

```bash
# Pull from Docker Hub or GHCR — pick one image or all
# Docker Hub
docker pull michaelobele/tomoshibi-api:latest    # api only (7431)
docker pull michaelobele/tomoshibi-mcp:latest    # mcp only (7433)
docker pull michaelobele/tomoshibi-web:latest    # web only (7432)
docker pull michaelobele/tomoshibi:latest        # all-in-one
# GHCR (same images)
docker pull ghcr.io/michael-obele/tomoshibi-api:latest
docker pull ghcr.io/michael-obele/tomoshibi-mcp:latest
docker pull ghcr.io/michael-obele/tomoshibi-web:latest
docker pull ghcr.io/michael-obele/tomoshibi:latest

# Run one service standalone (needs a running API on 7431)
docker run --rm -p 7431:7431 michaelobele/tomoshibi-api
docker run --rm -p 7433:7433 -e TOMOSHI_API_URL=http://host.docker.internal:7431 michaelobele/tomoshibi-mcp
# web bakes its API URL at image build time (http://api:7431), so run it via its
# compose file instead — that overrides the build arg: packages/web/docker-compose.yml

# Or via compose per-package
docker compose -f packages/api/docker-compose.yml up -d   # api + redis + searxng
docker compose -f packages/mcp/docker-compose.yml up -d   # mcp (needs api)
docker compose -f packages/web/docker-compose.yml up -d   # web (needs api)
```

### From source

```bash
git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi
go run ./packages/api/cmd/api              # needs Go 1.25+ and Chromium on PATH (7431)
REDIS_URL=redis://localhost:7434 go run ./packages/api/cmd/api   # with async
```

### Your first scrape

```bash
curl -X POST http://localhost:7431/v1/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

```json
{
  "url": "https://example.com",
  "markdown": "# Example Domain\n\n...",
  "metadata": { "title": "Example Domain" }
}
```

No `mode` needed — `smart` is default. `static` skips JS; `dynamic` always renders.

---

## Packages

| Package | Path                           | Description                                                                                    | Install                                                |
| ------- | ------------------------------ | ---------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| **api** | [`packages/api`](packages/api) | Go scraping API — Gin + Chromedp + Colly + native search roster (21 engines) | `docker compose up` or `go run ./packages/api/cmd/api` |
| **mcp** | [`packages/mcp`](packages/mcp) | MCP server — 3 tools, `tomoshi`/`tomoshibi` bin ([npm](https://www.npmjs.com/package/tomoshi)) | `npx -y tomoshi`                                       |
| **web** | [`packages/web`](packages/web) | Svelte 5 playground — scrape/crawl/search UI                                                   | `bun --cwd packages/web dev`                           |

```bash
# All packages
bun install          # root (if using workspaces)
# Or per-package
bun --cwd packages/mcp install && bun --cwd packages/mcp dev
bun --cwd packages/web install && bun --cwd packages/web dev
```

### Repository layout

```text
tomoshibi/
├── packages/
│   ├── api/            # Go scraping API — Gin + Chromedp + Colly + embedded Asynq worker (7431)
│   ├── mcp/            # MCP server — 3 tools over the API, npm package `tomoshi` (7433)
│   └── web/            # Svelte 5 playground — scrape/crawl/search in the browser (7432)
├── docs/               # API reference, search comparison (the rest is local-only)
├── skills/             # agent skills published to skills.sh (mirrored to .agents/, .agent/)
├── plan/               # local design plans (gitignored)
├── test_reports/       # historical test runs (gitignored)
├── .github/workflows/  # CI — ci.yml, docker.yml (images), npm.yml (publish)
├── docker-compose.yml  # full stack: api 7431 + redis 7434 + searxng 7435
├── package.json        # bun workspaces — dev:api · dev:mcp · dev:web · check
├── AGENTS.md           # agent context source of truth (CLAUDE.md symlinks to it)
├── README.md           # this file
└── LICENSE             # MIT
```

Each package has its own README with its own tree: [api](packages/api/README.md) · [mcp](packages/mcp/README.md) · [web](packages/web/README.md).

---

## Local telemetry

Every search records one event (who answered, fallbacks, latency, trace id) and every native engine attempt one more (ok/empty/blocked/timeout + reason) into **daily JSONL files that never leave the machine**:

```text
data/telemetry/search-2026-09-27.jsonl   # one line per search
data/telemetry/engine-2026-09-27.jsonl   # one line per engine attempt
```

- **View it:** `curl -s 'http://localhost:7431/v1/insights?hours=24' | jq` → per-engine scorecard (attempts, ok/empty/blocked, `last_error`, latency), chain stats (fallbacks, weak-result gates), p50/p95 latency, and `recent_errors` carrying `trace_id`s you can grep in `docker logs tomoshibi-api-1`.
- **Raw:** `cat data/telemetry/engine-*.jsonl | jq -s 'group_by(.engine) | map({engine: .[0].engine, n: length})'`
- **Config** (defaults): `TELEMETRY_ENABLED` (`true`) · `TELEMETRY_DIR` (`data/telemetry`) · `TELEMETRY_RETAIN_DAYS` (`14`, `0` = keep forever) — documented in `packages/api/.env.example`.
- **Docker:** compose bind-mounts `./data:/app/data`, so files survive recreation; `/data/` is gitignored.
- Off is instant: `TELEMETRY_ENABLED=false` → `/v1/insights` returns `{"enabled": false}`.

## Updating Docker

| You changed              | Do this locally                                                                                        | What CI does on push to `main`                                                     |
| ------------------------ | ------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| Go code (`packages/api`) | `docker compose up -d --build api`                                                                     | Docker workflow builds & pushes `ghcr.io/michael-obele/tomoshibi-*` (+ Docker Hub) |
| MCP (`packages/mcp`)     | `docker compose -f packages/mcp/docker-compose.yml up -d --build`                                      | npm workflow publishes `tomoshi` (auto-bump, e.g. 1.1.6 → 1.1.7)                   |
| Web (`packages/web`)     | `docker compose -f packages/web/docker-compose.yml up -d --build` — the backend URL is a **build** arg | same Docker workflow                                                               |
| compose / `.env` only    | `docker compose up -d` (no `--build`)                                                                  | —                                                                                  |

Release checklist: `make check` in `packages/api` (or `bun run check` in `packages/mcp`) → commit → push → watch `gh run list` → rebuild locally → verify:

```bash
docker logs tomoshibi-api-1 2>&1 | grep -E "Native search engines loaded|Telemetry recording"
curl -s localhost:7431/health
```

Self-hosters pull the same images CI pushed: `docker pull ghcr.io/michael-obele/tomoshibi-api:latest`.

## FAQ / troubleshooting

- **An engine stopped returning results.** Read the scorecard first: `curl -s 'localhost:7431/v1/insights?hours=24' | jq '.insights.engines'` → `blocked` / `last_error`. Confirm with a live probe (`CANARY_ENGINES=ddg go test -tags=canary ./internal/search/engines/ -run TestCanary -v` from `packages/api`), log the verdict in `plan/tomoshi-search/research.md` §3b, then fix `internal/search/engines/registry.default.yaml` (better `block_markers`, `proxy: webshare`, or `enabled: false`) and rebuild. **Roster rule: no edit without a live probe.**
- **A new error string shows up in logs.** Start from `recent_errors[].trace_id` in `/v1/insights` and grep that id in `docker logs`. The chain explains itself: `search: backend failed, trying next`, `search: results concentrated on one domain` (weak-result gate firing), `proxy budget exhausted — engines degraded to direct egress`.
- **My env var is ignored.** The api container takes an explicit `environment:` list (no `env_file`), and compose interpolation reads only the repo-root `.env` (gitignored). Add the var in **both** places, then `docker compose up -d` — use `--build` only when the code changed.
- **401 on `/v1/*`.** New builds honor `APP_API_KEYS`/`API_KEYS`; Docker passes neither, so auth is off there. If you enable it, also set `TOMOSHI_API_KEY` in your MCP config — otherwise every MCP call 401s.
- **Which search API keys do I need?** None; most engines are keyless. Optional quality boosts: `SERPER_API_KEY` (Google, 2,500 free), `TAVILY_API_KEY` (1,000 free/mo), `BRAVE_SEARCH_API_KEY` ($5/mo free credit). Set them in `packages/api/.env` **or** just in your MCP config; the MCP forwards whatever the backend asks for (`GET /v1/env`) on every request via `X-Tomoshi-Env`, so the key lives in one place (`TOMOSHI_FORWARD_ENV=false` opts out).
- **Port 7431 already in use.** An older container or process holds it (`docker ps`). Experiments use `SERVER_PORT=7452`.
- **`/v1/crawl`, `/v1/batch`, `/v1/monitor` return 503.** Redis is optional by design — scrape/search keep working; `docker compose up -d redis`.
- **MCP tools fail with `{}` or a raw `Invalid arguments …` dump.** You're on `tomoshi` < 1.1.6, which published a root-level `oneOf` schema clients rendered as empty properties. Upgrade/restart the MCP — 1.1.6+ ships flat schemas and readable usage errors.
- **Search results look thin or off-topic.** Check `backends` (who answered), `weak_gates` (how often native was single-domain and got merged with SearXNG/Brave hits), and `engines.<name>.empty` (corpus mismatches) in `/v1/insights`.

---

## API

Base: `http://localhost:7431` — every endpoint under `/v1`.

| Method | Endpoint              | What it does                                                                 | Needs Redis |
| ------ | --------------------- | ---------------------------------------------------------------------------- | ----------- |
| `POST` | `/v1/scrape`          | Single page → markdown (smart/static/dynamic, screenshots, `extract_schema`) | No          |
| `POST` | `/v1/scrape` + `urls` | Multi-URL sync (max 10, no Redis)                                            | No          |
| `POST` | `/v1/map`             | Sitemap → robots.txt → link fallback                                         | No          |
| `POST` | `/v1/search`          | Native 21-engine roster → SearXNG → stealth → Brave (keyed: brave/serper/tavily) | No   |
| `GET`  | `/v1/env`             | env var names a client may send per request in `X-Tomoshi-Env` (MCP forwarding) | No       |
| `POST` | `/v1/crawl`           | Async BFS crawl → `202` + job ID                                             | **Yes**     |
| `GET`  | `/v1/crawl/:id`       | Poll crawl status                                                            | **Yes**     |
| `POST` | `/v1/batch/scrape`    | Enqueue 20 URLs async                                                        | **Yes**     |
| `GET`  | `/v1/batch/:id`       | Poll batch                                                                   | **Yes**     |
| `POST` | `/v1/monitor`         | Hash markdown, webhook on change                                             | **Yes**     |

```
Client → Gin Router → Scraper (Colly / Chromedp + Readability)
                 ↘ Asynq (Redis) → Worker → same Scraper
                                         ↘ Browser Pool (one allocator, recycled every 100 scrapes)
```

---

## MCP

3 tools, not 8 — resource-oriented (`action` enum, ≤7 philosophy).

| Tool               | `action`                                                       | Endpoint                             |
| ------------------ | -------------------------------------------------------------- | ------------------------------------ |
| `tomoshi_extract`  | `scrape` · `scrape_multi` · `links` · `batch` · `batch_status` | `/v1/scrape`, `/v1/batch`            |
| `tomoshi_discover` | `search` · `map` · `crawl` · `crawl_status`                    | `/v1/search`, `/v1/map`, `/v1/crawl` |
| `tomoshi_monitor`  | `create` · `status` · `delete`                                 | `/v1/monitor`                        |

```jsonc
// .vscode/mcp.json or Claude config
{
  "mcpServers": {
    "tomoshi": {
      "command": "npx",
      "args": ["-y", "tomoshi"],
      "env": {
        "TOMOSHI_API_URL": "http://localhost:7431",
        // optional search keys — forwarded to the backend on every request
        // as X-Tomoshi-Env, so configure them HERE, not in the backend's .env
        "BRAVE_SEARCH_API_KEY": "BSB-...", // or SERPER_API_KEY / TAVILY_API_KEY
      },
    },
  },
}
```

The MCP asks the backend which key names it accepts (`GET /v1/env`) once, then forwards the matching ones from its own env on every call. `TOMOSHI_FORWARD_ENV` tunes it: `""` = all requested (default), `"false"` = off, or a comma list to restrict.

Also: `npx tomoshibi` (alias, same package).

---

## Web

Svelte 5 playground for scrape/crawl/search — same API, browser UI.

```bash
bun --cwd packages/web dev   # http://localhost:5173 (dev) / 7432 (docker)
```

Remote Functions proxy to the Go backend; API keys stay server-side.

---

## Agent Skills

Reusable instructions for AI coding agents — discoverable via [skills.sh](https://skills.sh) and the [`skills` CLI](https://github.com/vercel-labs/skills) (supports 75+ agents: Claude Code, Cursor, OpenCode, Codex, etc.).

This repo exposes skills from `skills/` (canonical, indexed by `skills.sh`) — mirrored to `.agents/skills/` and `.agent/skills/` for OpenCode/Cursor compatibility.

| Skill         | Path                                   | What it does                                                                                                    |
| ------------- | -------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| **tomoshibi** | [`skills/tomoshibi`](skills/tomoshibi) | Use Tomoshibi MCP (`tomoshi`) — `tomoshi_extract` / `tomoshi_discover` / `tomoshi_monitor` → LLM-ready markdown |

Verify discovery locally:

```bash
npx skills add ./ --list   # → tomoshibi
```

### Install globally (all projects)

```bash
# from GitHub (recommended) — installs to ~/<agent>/skills/ (e.g. ~/.claude/skills/, ~/.agents/skills/)
npx skills add Michael-Obele/tomoshibi -g

# only this skill (same when repo has one skill)
npx skills add Michael-Obele/tomoshibi --skill tomoshibi -g

# all skills, all detected agents, no prompts (CI-friendly)
npx skills add Michael-Obele/tomoshibi --all -g -y

# verify
npx skills list -g
```

### Install to this project only (team-shared)

```bash
# installs to ./.claude/skills/, ./.agents/skills/, etc. — commit to share with team
npx skills add Michael-Obele/tomoshibi

# specific agents
npx skills add Michael-Obele/tomoshibi -a claude-code -a opencode -a cursor

# from local checkout
npx skills add ./ --skill tomoshibi
```

### Use without installing

```bash
npx skills use Michael-Obele/tomoshibi --skill tomoshibi | claude
npx skills use Michael-Obele/tomoshibi --skill tomoshibi --agent claude-code
```

Browse on the web: [`skills.sh/Michael-Obele/tomoshibi`](https://skills.sh/Michael-Obele/tomoshibi) (after first push/index).

> **Adding a new skill:** create `skills/<name>/SKILL.md` with `name` + `description` frontmatter (see [`tomoshibi/SKILL.md`](skills/tomoshibi/SKILL.md)), then `npx skills add ./ --list` to verify. Keep `skills/` canonical — mirror to `.agents/skills/` / `.agent/skills/` if you need OpenCode compat.

---

## Benchmarks

| Stack         | Pull size | Running RAM | Containers |
| ------------- | --------- | ----------- | ---------- |
| **Tomoshibi** | ~1.79 GB  | ~317 MiB    | 4          |
| Firecrawl     | ~7.26 GB  | ~4.1 GiB    | 6          |

Measured 2026-08-31 (`docker images` + `docker stats`). **4× lighter, 13× less RAM.** Fits on 512 MB Fly.io / $5 Hetzner — Firecrawl doesn't.

---

## Name

**Tomoshibi** (灯火 — ともしび) — Japanese for _lamp light_, the glow of a paper lantern. Search as illumination: lighting up the web for AI agents. Short alias **tomoshi** (灯) for daily use. Formerly **Cinder** (extinct fire — wrong metaphor for discovery; `cinder` also collided with `github.com/cinder/Cinder` 5.5k★ and `npm/cinder`).

---

## License

MIT — see [LICENSE](LICENSE).

> **Migrating from Cinder?** `github.com/Michael-Obele/cinder` now redirects to `tomoshibi`. Docker image `ghcr.io/michael-obele/cinder` → `ghcr.io/michael-obele/tomoshibi`. No code changes needed beyond the import path (`github.com/Michael-Obele/tomoshibi`).
