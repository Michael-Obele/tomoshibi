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
  <a href="#why-tomoshi">Why tomoshi</a> ·
  <a href="#tools">Tools</a> ·
  <a href="#hosting">Hosting</a>
</p>

---

## Why tomoshi?

If you're paying Firecrawl/Exa per token or spawning a browser per request, you're overpaying.

| What hurts with hosted APIs | What tomoshi gives you | Outcome |
|---|---|---|
| **$0.01–0.10 per scrape** + rate limits | **$0 self-hosted** — one binary, hobby-tier RAM | Ship RAG without a cloud bill |
| **500ms Chrome spawn per request** | **One shared allocator + recycled tabs** | ~200ms static, parallel pools |
| **JS SPAs return empty HTML** | **Smart mode** — static first, fallback to Chromedp | Works on React/Vue without guessing |
| **Noisy HTML (nav/ads/footer)** | **Readability main-content** + ad block | Clean markdown your LLM wants |

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
        "TOMOSHIBI_API_URL": "http://localhost:7431"
      }
    }
  }
}
```

Legacy `CINDER_API_URL` still works. Aliases: `tomoshibi`, `tomoshibi-mcp` (same package).

Restart your editor — `tomoshi` appears as 3 tools.

---

## Tools

3 tools, not 8 — resource-oriented (`action` enum, ≤7 philosophy).

| Tool | `action` | What it does |
|---|---|---|
| `cinder_extract` | `scrape` · `scrape_multi` (≤10) · `links` · `batch` · `batch_status` | Single/multi-page → markdown, link extraction, async batch (Redis) |
| `cinder_discover` | `search` · `map` · `crawl` · `crawl_status` | SearXNG search (Brave fallback), sitemap, async BFS crawl |
| `cinder_monitor` | `create` · `status` · `delete` | Hash markdown, webhook on change |

Async actions (`batch`, `crawl`, `monitor`) are Redis-backed — poll `*_status` until done.

**Example — scrape + search in one turn:**

> `cinder_discover` `search` "svelte 5 runes" → `cinder_extract` `scrape_multi` on top 3 URLs → LLM-ready markdown

---

## Hosting

| Option | Command | Cost |
|---|---|---|
| **Docker Compose** | `docker compose up -d` | $0 (your machine) |
| **Fly.io** | `fly deploy` | ~$5/mo (512 MB) |
| **From source** | `go run ./packages/api/cmd/api` | $0 + Chromium |

API base: `http://localhost:7431` — all endpoints under `/v1` (`/scrape`, `/search`, `/crawl`, `/batch`, `/monitor`, `/map`).

---

## Links

- **GitHub:** https://github.com/Michael-Obele/tomoshibi
- **npm:** https://www.npmjs.com/package/tomoshi
- **API docs:** `https://your-api.fly.dev/docs` (Swagger, auto-generated)

---

## Name

**tomoshi** (灯 — ともし) — Japanese for *lamp, light*. Short form of **Tomoshibi** (灯火 — ともしび) *lamp light*, the glow of a paper lantern. Search as illumination: lighting up the web for AI agents.

## License

MIT — see [LICENSE](https://github.com/Michael-Obele/tomoshibi/blob/main/LICENSE).
