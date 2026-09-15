# Tomoshibi: The Svelte/JS Developer's Guide

> Looking for documentation for a specific part of the code? Check the [Documentation Index](INDEX.md).

Welcome to Tomoshibi! If you're coming from the JavaScript/TypeScript ecosystem (SvelteKit, Next.js, Node.js), this guide is designed to translate Go concepts into terms you already understand.

## 🗺️ Project Map (File Tree)

For a deeper dive into the architecture, check the [Architecture Guide](ARCHITECTURE.md).

```text
tomoshibi/
├── cmd/                        # 🚀 "scripts/" or "entry points"
│   ├── api/main.go             # The Hono/Express Server (monolith — API + Worker)
│   ├── worker/main.go          # The standalone Background Worker entry point
│   ├── demo-integration/       # Example client showing how to call the API
│   ├── e2e_test/               # End-to-end test binary
│   └── debug_asynq/            # Debug CLI for inspecting Asynq queues
├── internal/                   # 🔒 "src/" (Private code)
│   ├── api/                    # 🌐 API Routes & Middleware
│   │   ├── router.go           # Like SvelteKit hooks + routes setup
│   │   ├── handlers/           # Route handlers (like SvelteKit +server.ts)
│   │   ├── middleware/         # Logging, recovery middleware
│   │   └── docs/               # Auto-generated Swagger JSON
│   ├── config/                 # ⚙️ dotenv + Upstash Redis auto-config
│   ├── domain/                 # 📝 TypeScript Interfaces / Types (shared models)
│   │   ├── scraper.go          # ScrapeResult, ScrapeOptions, Scraper interface
│   │   └── media.go            # ImageData, ScreenshotData, BlobData
│   ├── scraper/                # 🧠 The Scraping Engine
│   │   ├── service.go          # The "orchestrator" — cache, mode dispatch
│   │   ├── colly.go            # Static scraper (like Cheerio)
│   │   ├── chromedp.go         # Dynamic scraper (like Puppeteer)
│   │   └── heuristics.go       # SPA detection — decides "static vs dynamic?"
│   ├── search/                 # 🔍 Web Search (Brave API integration)
│   │   └── service.go          # Search with rate limiting, domain filters
│   ├── image/                  # 🖼️ Image pipeline
│   │   ├── extractor.go        # HTML parsing → image metadata + base64
│   │   └── processor.go        # Image download, encoding, size limits
│   └── worker/                 # 👷 Async Job Queue (like BullMQ)
│       ├── server.go           # Asynq server setup + handler registration
│       ├── tasks.go            # Job type constants + payload structs
│       ├── handlers.go         # Single-page scrape task handler
│       └── crawl_handler.go    # Multi-page BFS crawl task handler
├── pkg/                        # 📦 Shared libraries
│   └── logger/                 # Structured logging (like Pino/Winston)
├── docs/                       # 📚 Documentation
│   ├── guides/                 # Developer guides (you are here!)
│   └── features/               # Feature deep-dives
├── Makefile                    # 🔧 Like package.json scripts (fmt, vet, test)
└── go.mod                      # 📦 package.json
```

---

## 🔄 The Rosetta Stone: Go vs JS

| Concept               | Go                                     | JavaScript / SvelteKit             |
| :-------------------- | :------------------------------------- | :--------------------------------- |
| **Server**            | `Gin`                                  | `Hono` / `Express`                 |
| **HTML Parser**       | `Colly`                                | `Cheerio`                          |
| **Headless Browser**  | `Chromedp`                             | `Puppeteer` / `Playwright`         |
| **Job Queue**         | `Asynq` + Redis                        | `BullMQ` + Redis                   |
| **Search**            | `Brave Search API`                     | `Brave Search API` (same!)         |
| **Image Pipeline**    | `internal/image/` (raw Go)              | Custom fetch → base64              |
| **Config**            | `Viper` + `.env`                       | `dotenv` + `$publicEnv`            |
| **Logging**           | `slog` structured logger               | `Pino` / `Winston`                 |
| **Concurrency**       | Goroutines (`go func()`)               | `Promise.all` / Web Workers        |
| **Type Contracts**    | Implicit Interfaces (`type X interface`) | TypeScript Interfaces             |
| **Project Layout**    | Standard Go (`cmd`, `internal`, `pkg`) | SvelteKit (`src/routes`, `$lib`)   |

---

## 🌊 The Flow: Life of a Request

Let's follow a request to `POST /v1/scrape`.

### 1. The Entry Point (`cmd/api/main.go`)

This is like your `server.ts`. It loads the config (`.env`), connects to Redis, initializes the services, and starts the HTTP server.

- **Key Detail**: It runs in "Monolith Mode" by default now. It spins up the **Worker** in a background Goroutine so you don't need a separate container during development!.

### 2. The Router (`internal/api/router.go`)

This sets up the routes.

```go
v1.POST("/scrape", scrapeHandler.Scrape)
```

Equivalent to SvelteKit's `export const POST = ...`.

### 3. The Handler (`internal/api/handlers/scrape.go`)

This verifies the input (JSON body).

- **Validates**: Checks if `url` is present.
- **Maps**: Converts "render: true" to "mode: dynamic" for backward compatibility.
- **Calls Service**: Passes control to the detailed logic layer.

### 4. The Service (`internal/scraper/service.go`)

This is the "Brain". It decides _how_ to scrape.

1.  **Check Cache**: Looks in Redis for `scrape:<url>:<mode>:<screenshot>:<images>`. If found (gzip-compressed), returns immediately (fast!).
2.  **Mode Switch**:
    - **Static** — Uses `Colly` (HTTP Request + HTML parsing). Fast, low resource usage.
    - **Dynamic** — Uses `Chromedp` (Headless Chrome). Spins up a browser tab, executes JS. Slower, but accurate for SPAs.
    - **Smart (default)** — Tries Static first. Then runs heuristics (see below). If the HTML looks like an SPA shell, it falls back to Dynamic.
3.  **Image / Screenshot Handling**: If `screenshot: true`, dynamic mode is forced. Screenshots are captured as full-page base64 JPEGs. If `images: true`, the HTML is parsed for `<img>` tags and OG/twitter meta images.
4.  **Cache Write**: Results are gzip-compressed and stored in Redis for 7 days.

### 5. The Smart Heuristics (`internal/scraper/heuristics.go`)

This is what makes "smart" mode intelligent. It analyzes the HTML from the static scrape and decides if a dynamic (headless) scrape is needed:

- **`<noscript>` checks** — Looks for "enable JavaScript" messages inside noscript tags.
- **SPA root detection** — Scans for `id="root"`, `id="__next"`, `data-reactroot`, `__NEXT_DATA__`, Angular roots.
- **Content size heuristic** — If the body is tiny (< 2KB) but has `<script>` tags, it's likely a JS shell.

> **Svelte comparison**: Think of this like checking if a page's SSR output is empty because it's a client-rendered SvelteKit SPA. If we only got the `<div id="svelte">` shell from static HTML, we'd know to use headless Chrome.

### 6. The Scraper (`internal/scraper/`)

- `colly.go`: The "Cheerio" wrapper. Uses rotating user-agents via `gofakeit.UserAgent()`.
- `chromedp.go`: The "Puppeteer" wrapper. Instead of launching a full browser per request, it connects to a **shared browser instance** (the Allocator Context) and just opens a new tab per request.

### 7. Search Service (`internal/search/service.go`)

Like a SvelteKit `+server.ts` that proxies an external API. It calls the **Brave Search API**, with:
- **Rate limiting**: 1 request per 1.1 seconds to avoid 429s.
- **Pagination**: `offset`/`limit` support.
- **Domain filtering**: `includeDomains` / `excludeDomains`.
- **Freshness**: `maxAge` for filtering recent results.

### 8. Image Pipeline (`internal/image/`)

Two files handle the image features:
- **`extractor.go`** — Parses HTML with `goquery` (like jQuery). Finds OG meta images, Twitter card images, and `<img>` tags in priority order.
- **`processor.go`** — Downloads images from URLs, enforces size limits (5MB), encodes to base64 data URIs (`data:image/png;base64,...`). These are ready to feed directly to AI APIs (OpenAI, Gemini, Claude).

---

## 🏗️ Architecture Improvements

We recently refactored the project to be friendlier for "Hobby" deployments (like Railway, Render, or Leapcell).

### The Monolith Pattern

Previously, you needed to run two commands:

1. `go run cmd/api/main.go`
2. `go run cmd/worker/main.go`

Now, **`cmd/api/main.go` does both!**
It checks if `DISABLE_WORKER` is false (default), and initializes the `asynq` server inside the same process. This means "Scale to Zero" works perfectly—one container handles the API _and_ processes background crawl jobs.

### Concurrency

We tuned the concurrency limit to **10** (up from 2). Because we use shared browser contexts instead of spawning new chrome processes, memory usage is stable (~500MB), allowing higher throughput on small VPS instances.

---

## 🚀 Running Locally

You only need one command now:

```bash
go run cmd/api/main.go
```

This starts:

- HTTP API on `:8080`
- Background Worker (listening to Redis)

**Test it:**

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -d '{"url": "https://example.com", "mode": "static"}'
```

> **🔥 Pro-Tip for Svelte Devs:** For a comprehensive guide on how to integrate this backend with your Vite/Svelte frontend, as well as how to **Test** and **Debug** using VS Code, see the [Svelte Dev Workflow Guide](SVELTE_DEV_WORKFLOW.md).

---

_This guide was generated by analyzing the codebase with `mcp_sequentialthi_sequentialthinking` and verified against the running application._
