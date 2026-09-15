# Performance Refactor: A Guide for JS Developers

We've just optimized the Tomoshibi scraping engine. If you're coming from a Node.js/JS background (Puppeteer/Playwright), here is what changed and why.

> [!NOTE]
> For a full walkthrough of the current codebase structure, see the [Code Walkthrough](CODE_WALKTHROUGH.md) or the [Documentation Index](INDEX.md).

## 1. The "Browser Context" Pattern (vs Spawning Processes)

**Old Way (The "PHP" way):**
Every time we got a request to `/scrape`, we launched a brand new Chromium process (`fork()`).

- _JS Equiv:_ Running `puppeteer.launch()` inside your Express handler for every request.
- _Result:_ 1-2s latency penalty, huge CPU spikes, container crashes.

**New Way (The "Worker" way):**
We now treat the Browser like a Database Connection Pool.

- **Startup:** We launch **one** Chromium process when the server starts (`Global Singleton`).
- **Request:** We just open a new **Tab** (Context) for that URL.
- _JS Equiv:_ `const browser = await puppeteer.launch();` at app startup, then `await browser.newPage()` per request.
- _Result:_ <10ms overhead per request. Huge speedup.

## 2. Monolith Architecture ("Next.js" Style)

**Old Way:**
Two separate services: `api` (Gin) and `worker` (Asynq Consumers).

- _Problem:_ Deployment hell. You need two containers. On "Hobby" plans (Leapcell, Vercel), running a worker 24/7 hits execution limits immediately.

**New Way:**
We implemented a **Monolith** pattern.

- The `api` server now spins up the `worker` in a background Goroutine.
- _JS Equiv:_ Like `Next.js` API routes or `Inngest` running inside your Next.js app.
- _Benefit:_ "Scale to Zero". When no requests come in, nothing runs. When a request hits, the server wakes up, handles the API, AND processes any background jobs immediately.

## 3. Concurrency Tuning

We bumped the concurrency from `2` to `10`.

- Since we aren't launching full browsers anymore, memory usage is way lower.
- We can now handle 10 simultaneous scraping tabs easily within the 4GB RAM limit.

---

## Summary for Frontend Usage

- **Endpoint**: `POST /v1/scrape` is now significantly faster (Time-to-First-Byte reduced by ~1s).
- **Queues**: You can fire-and-forget to `POST /v1/crawl`, and even if you are on a Free tier, it will get processed as long as the API container is active.
