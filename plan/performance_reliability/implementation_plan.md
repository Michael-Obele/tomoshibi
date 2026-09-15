# Performance & Reliability Implementation Plan

## 🚀 Goal

Transform Tomoshibi from a "PoC" scraper into a **production-grade**, high-throughput engine capable of running efficiently on serverless/container infrastructure.

---

## Step 1: Shared Browser Allocator (The Big Fix)

Refactor `ChromedpScraper` to separate "Browser Lifecycle" from "Scrape Request".

### Changes Required: `internal/scraper/chromedp.go`

1.  **Struct Update**: Add `allocCtx` and `cancelAlloc` to the `ChromedpScraper` struct.
2.  **Initialization**:
    - Create a `Start()` or `Init()` method (or put in `NewChromedpScraper`).
    - Initialize `NewExecAllocator` **once**.
    - Initialize a parent `NewContext` (the browser).
3.  **Execution**:
    - In `Scrape(ctx, url)`: Use `chromedp.NewContext(s.allocCtx)` to create a **tab**.
    - This allows fast tab creation/destruction without process overhead.
4.  **Cleanup**: Add a `Close()` method to shut down the browser cleanly on app exit.

### Code Sketch

```go
type ChromedpScraper struct {
    rootCtx context.Context // The allocator context
    cancel  context.CancelFunc
}

func NewChromedpScraper() *ChromedpScraper {
    opts := [...] // flags
    allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
    // Start the browser immediately so we fail fast if missing
    ctx, _ := chromedp.NewContext(allocCtx)
    if err := chromedp.Run(ctx); err != nil { ... }
    return &ChromedpScraper{ rootCtx: ctx, cancel: cancel }
}

func (s *ChromedpScraper) Scrape(...) {
    // Create a tab from rootCtx
    ctx, cancel := chromedp.NewContext(s.rootCtx)
    defer cancel()
    // ... run tasks ...
}
```

---

## Step 2: "Smart" Waiting & Bot Evasion

Improve the quality of the scrape and avoid getting blocked.

### Changes Required: `internal/scraper/chromedp.go`

1.  **User-Agent**: Inject the `gofakeit` generic User-Agent into the `chromedp` options or network headers.
2.  **Wait Logic**:
    - Add `WaitFor` parameter to `ScrapeRequest`.
    - Implement a default "Wait for Network Idle" helper (using `chromedp.ActionFunc`).
    - Fallback: `time.Sleep` if `waitFor` is specified in request.

---

## Step 3: Deployment Topology (Monolith Mode)

**Context**: Leapcell Hobby gives **4GB RAM** but limited **Execution Time**. Running separate containers is wasteful.

### Changes Required: `cmd/api/main.go`

1.  **Integration**: Initialize the `Asynq` Server _inside_ the API `main.go`.
2.  **Goroutine**: Run the Asynq server in a separate goroutine.
3.  **Config**: Add a flag `ENABLE_WORKER=true` to control this behavior.
4.  **Benefit**: Single container handles HTTP requests and processes the resulting queue immediately. Best for "Scale-to-Zero" environments.

---

## Step 4: Worker & Queue Tuning

Maximize throughput within the 4GB RAM envelope.

### Changes Required:

1.  **Concurrency**: Increase to **10**. (4GB is plenty for 10 tabs).
2.  **Polling**: Reduce `TaskCheckInterval` to `1s` for snappier response.

---

## Step 5: Resilience & Memory Safety

Prevent crashes and leaks.

### Changes Required:

1.  **Browser Restart**: Chrome gets "tired" (memory fragmentation) after many pages.
    - **Strategy**: Implement a `requestsProcessed` counter in `ChromedpScraper`.
    - If `count > 100`, trigger a `RestartBrowser()` method to kill and respawn the base allocator.
2.  **OOM Protection**: Leapcell will kill the container if we exceed memory.
    - Set `chromedp` flag `--disable-dev-shm-usage` (already done, good).
    - Monitor memory (optional, via separate task).

---

## 📅 Execution Order

1.  **Refactor `ChromedpScraper`** (High ROI).
2.  **Update Worker Config** (Easy).
3.  **Enhance Wait Logic** (Quality).
4.  **Add Browser Restart Logic** (Stability).
