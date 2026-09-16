# 🛠️ Svelte Developer Workflow: Run, Use, Test & Debug

Welcome to the day-to-day workflow guide! As a Svelte developer working on Tomoshibi, you're bridging the gap between a robust Go backend and a sleek Svelte/JS frontend ecosystem. This guide tells you exactly how to spin things up, handle integrations, and troubleshoot when things go wrong.

> [!IMPORTANT]
> This guide focuses on **daily development tasks**. For a deeper understanding of the code, see the [Go for Svelte Devs](GO_FOR_SVELTE_DEVS.md) guide and the [Documentation Index](INDEX.md).

---

## 🏃 1. How to Run the Project

The Tomoshibi project uses a **monolith pattern** for local development. You do not need to start up five different microservices.

### Running the Go Backend

The backend contains both the API server and the background worker.

```bash
# From the project root
go run cmd/api/main.go
```

**What this does:**

- Starts the HTTP API on `http://localhost:8080`
- Automatically starts the background worker (listening to Redis) within the same process.
- Hot-reloading is not built into Go by default (unlike Vite). If you make a change to a `.go` file, you need to stop (`Ctrl+C`) and re-run the command.
  - _Pro-tip:_ Install `air` (`go install github.com/cosmtrek/air@latest`) and just run `air` in the terminal for hot-reloading!

### Running the Frontend / JS Services

If you are working in `tomoshibi-js` or a SvelteKit consuming app:

```bash
# In your Svelte/JS directory
npm install
npm run dev
```

This runs your standard Vite dev server, typically on `http://localhost:5173`.

---

## 💻 2. How to Use (Consuming the API)

As a Svelte developer, your main interaction with the Go backend will be via `fetch` calls, typically inside `+page.server.ts` or `+server.ts` files.

### Example: Calling the Scrape API from SvelteKit

```typescript
// src/routes/dashboard/+page.server.ts
import type { PageServerLoad } from "./$types";

export const load: PageServerLoad = async ({ fetch }) => {
  // 1. Hit the local Go backend
  const response = await fetch("http://localhost:8080/v1/scrape", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      url: "https://example.com",
      mode: "smart", // 'static', 'dynamic', or 'smart'
      screenshot: false, // Set true to get a base64 screenshot blob
      images: false, // Set true to extract page images as base64
    }),
  });

  if (!response.ok) {
    const errorData = await response.json();
    console.error("Tomoshibi API Error:", errorData);
    return { error: "Failed to scrape" };
  }

  const data = await response.json();

  // 2. Pass the Markdown/HTML + optional media to the Svelte component
  return {
    markdown: data.markdown,
    metadata: data.metadata,
    screenshot: data.screenshot, // { blob, format, width, height, full_page, truncated }
    images: data.images, // [{ url, blob, alt, width, height, source }]
  };
};
```

### Example: Calling the Search API from SvelteKit

```typescript
// src/routes/search/+page.server.ts
import type { PageServerLoad } from "./$types";

export const load: PageServerLoad = async ({ fetch, url }) => {
  const query = url.searchParams.get("q") || "svelte 5 runes";

  const response = await fetch(
    `http://localhost:8080/v1/search?q=${encodeURIComponent(query)}&limit=10`,
  );

  if (!response.ok) {
    return { error: "Search failed", results: [] };
  }

  const data = await response.json();

  return {
    query: data.query,
    results: data.results, // [{ title, url, description, domain, relevance }]
    hasMore: data.hasMore,
    nextOffset: data.nextOffset, // Pass this back for "load more"
  };
};
```

> **🔍 Search requires a `BRAVE_SEARCH_API_KEY`** environment variable. Sign up at [Brave Search API](https://brave.com/search/api/).

---

## 🧪 3. How to Test

You live in two worlds: the Go backend and the JS/Svelte frontend.

### Quality Checks (`make check` — like `npm run check`)

The project has a `Makefile` that bundles all quality checks into one command:

```bash
# Run everything: format + lint + static analysis + tests
make check

# Or run individually:
make fmt          # go fmt (like Prettier for Go)
make vet          # go vet  (like basic ESLint)
make staticcheck  # staticcheck (like advanced ESLint — install with: go install honnef.co/go/tools/cmd/staticcheck@latest)
make test         # go test -v (like Vitest)
```

### Testing the Go Backend

There is no Jest or Vitest here. Go has built-in testing.

```bash
# Run all backend tests across the project
go test ./... -v

# Run tests without caching (if you suspect stale results)
go test ./... -count=1 -v

# Run a specific test suite (e.g., scraper package)
go test ./internal/scraper/... -v

# Run with race detection (critical for concurrent code)
go test -race ./...

# Run with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

**Mental mapping:**

- `describe()` / `it()` -> `func TestSomething(t *testing.T) { t.Run(...) }`
- `expect(x).toBe(y)` -> `if x != y { t.Errorf(...) }`
- The test file sits right next to the code: `scrape.go` → `scrape_test.go`

_For more in-depth testing setup, check out the [Testing Guide](TESTING.md)._

### Testing the Svelte/JS Side

Business as usual!

```bash
# In your Svelte project
npm run test:unit      # Vitest
npm run test:ui        # Vitest UI
npm run test:e2e       # Playwright
```

---

## 🐛 4. How to Debug

Debugging cross-stack can be tricky. Here is how to find out why things are breaking.

### Debugging Go with VS Code

Don't just use `fmt.Println` (the Go equivalent of `console.log`). Use the debugger!

1. Install the **Go extension** in VS Code.
2. Open the "Run and Debug" panel (Ctrl+Shift+D).
3. Click "create a launch.json file" and select "Go".
4. Replace the contents of `.vscode/launch.json` with:
   ```json
   {
     "version": "0.2.0",
     "configurations": [
       {
         "name": "Launch Tomoshibi API",
         "type": "go",
         "request": "launch",
         "mode": "auto",
         "program": "${workspaceFolder}/cmd/api/main.go",
         "env": {
           "LOG_LEVEL": "debug"
         }
       }
     ]
   }
   ```
5. Set breakpoints in your `.go` files by clicking the gutter.
6. Hit **F5**. The API server will start, and VS Code will pause execution on your breakpoints, allowing you to inspect variables just like in Chrome DevTools!

### Logging Fallback (`console.log` equivalent)

If you must "console.log" something quickly in Go, use the structured logger instead of `fmt.Println`:

```go
import "github.com/standard-user/tomoshibi/pkg/logger"

// Equivalent to console.log("Data:", myVar)
logger.Log.Info("Debugging", "myVar", myVar)

// Equivalent to console.error("Error:", err)
logger.Log.Error("Something broke", "error", err)
```

Make sure your server is running with `LOG_LEVEL=debug` if you are using `logger.Log.Debug()`.

### Upstash Redis (Free Tier Friendly)

You don't need a local Redis server. Tomoshibi can auto-configure Upstash Redis from the standard env vars:

```bash
# In your .env file — just paste from Upstash console:
UPSTASH_REDIS_REST_URL=https://your-instance.upstash.io
UPSTASH_REDIS_REST_TOKEN=your_token_here
```

The app automatically derives `rediss://default:token@host:6379` from these. This means zero-config Redis on free-tier Upstash.

### Common Gotchas

- **CORS Errors**: If your SvelteKit frontend (running in the browser) tries to call `localhost:8080/v1/scrape` directly via a client-side `fetch()`, you might get CORS errors. **Always proxy requests through your SvelteKit `+server.ts` endpoints or use `+page.server.ts`** to make server-to-server calls.
- **Port Conflicts**: If `localhost:8080` is taken, set `PORT=8081` in your `.env` file for the Go backend.
- **Nil Pointers**: The equivalent of "Cannot read properties of undefined". If Go panics with "invalid memory address or nil pointer dereference", look for a variable that is a pointer (has a `*` in its type) that was never initialized.
- **Worker in debug**: The embedded worker (monolith mode) polls Redis every 15s. Be patient after enqueuing a crawl job — it can take 15s to be picked up. Set `DISABLE_WORKER=true` to turn it off if you only need the synchronous scrape API.
- **Swagger auto-gen**: In `debug` mode, the server automatically regenerates Swagger docs on start. If you add a new endpoint, just restart the server and check `/swagger/index.html`.
