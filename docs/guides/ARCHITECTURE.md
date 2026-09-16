# Tomoshibi Architecture Guide (For JS/SvelteKit Developers)

> [!TIP]
> New here? Start with the [Documentation Index](INDEX.md) for a guided tour.

Welcome to the Tomoshibi codebase! If you're coming from a SvelteKit/Node.js background, some Go patterns might feel "different." This guide bridges that gap by comparing Go's structure and syntax to the JS ecosystem you already know.

## 🏗 High-Level Architecture

Tomoshibi is a Go-based distributed scraper (a lightweight alternative to Firecrawl). It uses a **Task Queue** pattern to handle long-running scrapes without blocking the main API. For a more detailed look at the project layout, see the [Project Tour](PROJECT_TOUR.md).

### Project Layout (Standard Go)

- `/cmd`: Entry points (think of these as your `src/routes/api` endpoints or separate microservices).
  - `api/main.go`: The web server.
  - `worker/main.go`: The background process.
- `/internal`: Private code. In Go, code here cannot be imported by other projects. This is where the core logic lives.
- `/pkg`: Shared, reusable code (like a local utility library).
- `/internal/domain`: The "Source of Truth." This defines the interfaces (like TypeScript types) that everyone must follow.

---

## 🔄 The "JS to Go" Rosetta Stone

| Concept           | JS/SvelteKit Equivalent        | How it works in Tomoshibi                                           |
| :---------------- | :----------------------------- | :------------------------------------------------------------------ |
| **Interfaces**    | TypeScript `interface` or Type | Decouples code. Any struct with a `Scrape()` method is a `Scraper`. |
| **Gin**           | Express / Hono                 | The web framework used for routing and middleware.                  |
| **Colly**         | Cheerio / Axios                | Fast, static HTML scraper.                                          |
| **Chromedp**      | Puppeteer / Playwright         | Uses Headless Chrome for JS-heavy dynamic sites.                    |
| **Asynq + Redis** | BullMQ + Redis                 | Manages the background "Crawl" jobs.                                |
| **Goroutines**    | `Promise.all` / `async`        | Go's way of doing concurrent work (lightweight threads).            |
| **slog**          | Pino / Winston                 | Structured logging for debugging.                                   |

---

## 🛣 Control Flow: Life of a Request

### 1. Synchronous Scrape (`POST /v1/scrape`)

Equivalent to a standard `await`ed API call in SvelteKit.

1. `router.go` receives the request.
2. `handlers/scrape.go` validates the input (url required, optional screenshot/images flags).
3. `scraper/service.go` (The Orchestrator) decides:
   - **Static** → `colly.go` (fast, like Cheerio)
   - **Dynamic** → `chromedp.go` (headless Chrome, like Puppeteer)
   - **Smart (default)** → Try static first, run `heuristics.go` to check if the HTML is an SPA shell, fall back to dynamic if needed
4. If `screenshot: true`, dynamic mode is forced and a full-page JPEG is captured.
5. If `images: true`, `extractor.go` parses `<img>` tags, and `processor.go` downloads + base64-encodes them.
6. Result is gzip-compressed and cached in Redis for 7 days, then returned.

### 2. Web Search (`POST /v1/search`)

Like a SvelteKit `+server.ts` that proxies an external API.

1. `handlers/search.go` validates the query.
2. `search/service.go` calls the **Brave Search API** with rate limiting (1 req / 1.1s).
3. Filters, paginates, and returns results with relevance scoring.

### 3. Asynchronous Crawl (`POST /v1/crawl`)

Equivalent to pushing a job to **BullMQ** and returning a Job ID.

1. `handlers/crawl.go` enqueues a task to **Redis** via Asynq.
2. API returns an `id` immediately (HTTP 202 Accepted).
3. The **Worker** (embedded in the API process in monolith mode, or standalone `cmd/worker/main.go`) picks up the task.
4. `worker/crawl_handler.go` performs a **BFS crawl**: scrapes the seed URL, extracts links, follows same-domain links up to `maxDepth`.
5. Results are stored in Asynq's result backend — poll `GET /v1/crawl/:id` to check status.

---

## 🛠 Patterns Used

### Dependency Injection (Manual)

In Go, we "wire up" our code in `main.go`. We create the Scrapers first, then pass them into the Service, which is then passed into the Handler. This is like passing props in Svelte, but for business logic.

### Interface-Based Design

Look at `internal/domain/scraper.go`. It defines what a "Scraper" is:

```go
type Scraper interface {
    Scrape(ctx context.Context, url string, opts ScrapeOptions) (*ScrapeResult, error)
}
```

Because of this, we can easily swap Colly for Chromedp, or add a third "AI Scraper" without changing the API logic. This is like Svelte's `interface` pattern — as long as the component accepts the same props, you can swap implementations.

### Smart Heuristics (`internal/scraper/heuristics.go`)

The "smart" mode isn't magic — it's a plain function that analyzes HTML. Think of it like SvelteKit's `server-side rendering check`: if the static HTML we got back looks like an empty SPA shell (just `<div id="root">` with `<script>` tags), we know we need the headless browser. It checks:

- `<noscript>` "enable JavaScript" messages
- Framework-specific root elements (`id="__next"`, `data-reactroot`, `<app-root>`, etc.)
- Tiny HTML + lots of `<script>` tags (likely JS-only shell)

---

## 💻 Dev & Build Guide

### Prerequisites

Before you start, you'll need:

- **Go 1.22+**: Download from [golang.org](https://golang.org/dl/)
- **Redis**: For the async task queue and caching
  - Local: Install Redis server (`brew install redis` on macOS, `apt install redis-server` on Ubuntu)
  - Or use Upstash free tier (set `UPSTASH_REDIS_REST_URL` + `UPSTASH_REDIS_REST_TOKEN` in `.env` — the app auto-derives the TLS Redis URL)
- **Brave Search API Key** (optional): For the `/v1/search` endpoint. Get one at [brave.com/search/api/](https://brave.com/search/api/)
- **Git**: For cloning and version control

### Environment Setup

1. **Clone the repository**:

   ```bash
   git clone <your-repo-url>
   cd tomoshibi
   ```

2. **Install dependencies**:

   ```bash
   go mod tidy
   ```

3. **Create environment file** (copy from example):

   ```bash
   cp packages/api/.env.example .env   # root .env for `docker compose` (also works as packages/api/.env for `go run`)
   ```

   Edit `.env` with your values (see `packages/api/.env.example` for all options):

   ```bash
   REDIS_URL=redis://localhost:7434
   SEARXNG_ENDPOINT=http://localhost:7435
   ```

4. **Start Redis** (if running locally):
   ```bash
   redis-server
   ```

### Building the Project

#### Build Commands

```bash
# Build the API server
go build -o bin/tomoshibi-api cmd/api/main.go

# Build the async worker
go build -o bin/tomoshibi-worker cmd/worker/main.go

# Build both at once
go build ./cmd/...
```

#### Cross-Compilation (Optional)

```bash
# Build for Linux (common for deployment)
GOOS=linux GOARCH=amd64 go build -o bin/tomoshibi-api-linux cmd/api/main.go

# Build for different architectures
GOOS=darwin GOARCH=arm64 go build -o bin/tomoshibi-api-mac-arm64 cmd/api/main.go
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for specific package
go test ./internal/scraper/...

# Run tests with verbose output
go test -v ./...

# Run tests with race detection
go test -race ./...
```

### Running Locally

#### Development Mode (API + Worker)

1. **Start the API server**:

   ```bash
   go run cmd/api/main.go
   ```

   Server starts on `http://localhost:8080`

2. **Start the async worker** (in another terminal):
   ```bash
   go run cmd/worker/main.go
   ```

#### Testing the API

```bash
# Test synchronous scraping
curl -X POST http://localhost:8080/v1/scrape \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-key" \
  -d '{"url": "https://example.com", "renderJS": false}'

# Test asynchronous crawling
curl -X POST http://localhost:8080/v1/crawl \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-key" \
  -d '{"url": "https://svelte.dev", "maxPages": 5}'

# Check crawl status (replace JOB_ID with actual ID)
curl http://localhost:8080/v1/crawl/JOB_ID
```

#### Development Workflow

1. **Make changes** to code
2. **Run tests**: `go test ./...`
3. **Build**: `go build ./cmd/...`
4. **Run locally**: `go run cmd/api/main.go` (and worker in another terminal)
5. **Test endpoints** with curl or Postman
6. **Check logs** for debugging
7. **Commit and push** changes

### Building with Docker

The `Dockerfile` uses a **multi-stage build**:

1. **Build stage**: Compiles Go code into optimized binaries
2. **Runtime stage**: Creates minimal container with Chromium for dynamic scraping

```bash
# Build the Docker image
docker build -t tomoshibi .

# Run the container
docker run -p 8080:8080 --env-file .env tomoshibi
```

### Docker Compose (Full Stack)

```bash
# Start everything (API, Worker, Redis)
docker compose up --build

# Start in background
docker compose up -d

# View logs
docker compose logs -f api
docker compose logs -f worker

# Stop everything
docker compose down
```

### Useful Commands

- `go mod tidy`: Clean up dependencies
- `go mod vendor`: Vendor dependencies (for Docker builds)
- `go fmt ./...`: Format all Go code
- `go vet ./...`: Static analysis for bugs
- `go mod graph`: View dependency graph
- `docker compose logs -f worker`: Watch worker process jobs
- `redis-cli monitor`: Watch Redis commands in real-time

---

## 🚀 Deployment on Leapcell

Leapcell is a serverless platform perfect for Go applications with async workers like Tomoshibi. It offers pay-as-you-go pricing - you only pay when your service is processing requests.

### Prerequisites

- GitHub account
- Leapcell account ([leapcell.io](https://leapcell.io))

### Deployment Steps

1. **Push your code to GitHub**:

   ```bash
   git add .
   git commit -m "Ready for deployment"
   git push origin main
   ```

2. **Connect GitHub to Leapcell**:
   - Go to [Leapcell Dashboard](https://leapcell.io/dashboard)
   - Follow instructions to connect your GitHub account

3. **Create a new service**:
   - Click "New Service"
   - Select your repository from the list
   - Configure the service:

   | Field             | Value                                                                                    |
   | ----------------- | ---------------------------------------------------------------------------------------- |
   | **Runtime**       | Go (Any version)                                                                         |
   | **Build Command** | `chmod +x install_browser.sh && ./install_browser.sh && go build -o app cmd/api/main.go` |
   | **Start Command** | `export PATH=$PATH:. && ./app`                                                           |
   | **Port**          | `8080`                                                                                   |

   **Note**: The `install_browser.sh` script installs a headless Chrome browser in the build environment and moves it to the application directory so `chromedp` can find it.

4. **Environment Variables**:
   Add these in Leapcell dashboard:
   - `REDIS_URL`: Your Redis connection string (use Leapcell Redis or Upstash)
   - `API_KEY`: Your API key for authentication

### Redis Configuration for Leapcell

Leapcell provides managed Redis. For TLS connections (required for Leapcell/Upstash Redis):

```bash
# In your .env or Leapcell env vars
REDIS_URL=rediss://username:password@host:port
```

The code handles TLS automatically (see `internal/config/redis.go`).

### Accessing Your Deployed App

Once deployed, Leapcell provides a URL like `your-app-name.leapcell.dev`

Test your deployment:

```bash
curl -X POST https://your-app-name.leapcell.dev/v1/scrape \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{"url": "https://example.com"}'
```

### Scaling and Async Workers

For production with heavy async workloads, consider:

- **Separate Worker Service**: Deploy the worker (`./tomoshibi-worker`) as a separate Leapcell service
  - **Build Command**: `chmod +x install_browser.sh && ./install_browser.sh && go build -o worker cmd/worker/main.go`
  - **Start Command**: `export PATH=$PATH:. && ./worker`
- **Auto-scaling**: Leapcell scales automatically based on traffic
- **Monitoring**: Check Leapcell dashboard for logs, metrics, and performance

### Continuous Deployments

Every push to the connected branch automatically triggers a build and deploy. Failed builds are safely rolled back, keeping your service running.

### Cost Optimization

Leapcell's pay-as-you-go model is perfect for scrapers:

- **Free quota**: Generous monthly free tier
- **No idle costs**: Only pay for actual request processing
- **Auto-scaling**: Scales down to zero when not in use

### Troubleshooting

- **Build failures**: Check Leapcell logs for Go compilation errors
- **Runtime errors**: Verify environment variables and Redis connectivity
- **TLS issues**: Ensure Redis URL uses `rediss://` for secure connections
- **Worker issues**: If using separate worker service, check its logs too

For help, join the [Leapcell Discord community](https://discord.gg/qF7efny8x2).

---

## 🚀 Future Insight: Suggestions for You

As a SvelteKit dev, you might notice:

1. **Frontend**: We could build a SvelteKit Dashboard that hits `/v1/crawl` and shows status in real-time.
2. **Websockets**: We could add a websocket layer to notify the frontend when a crawl is finished.
3. **Storage**: Currently, crawl results are logged. We should eventually save them to a DB (Postgres/Supabase) so your SvelteKit app can fetch them easily.
