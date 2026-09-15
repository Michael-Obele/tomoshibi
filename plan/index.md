---
title: Tomoshibi (Go Scraper Backend)
status: planning
owner: standard-user
tags: [go, gin, scraping, colly, chromedp, firecrawl, redis, asynq, tomoshibi]
---

# Tomoshibi (Go Scraper Backend)

A high-performance, self-hosted web scraping API built with Go. **Tomoshibi** is designed to be a drop-in alternative to Firecrawl, capable of turning any website (static or JS-rendered) into LLM-ready markdown.

## 🎯 Goal

Build a robust scraping service that can:

1.  **Scrape**: Extract clean Markdown from any URL.
2.  **Render**: Handle complex JavaScript/SPA sites (React, Vue, etc.) using a headless browser.
3.  **Queue**: Manage heavy crawl jobs asynchronously using Redis.
4.  **Scale**: Deploy easily via Docker with low memory footprint.
5.  **Evade**: Rotate User Agents automatically to avoid bot detection.

## 🛠️ Tech Stack & Rationale

Since you are new to Go, we have chosen "boring" (stable, popular) technology to minimize technical debt.

| Component            | Technology                                                                      | Why?                                                                           |
| :------------------- | :------------------------------------------------------------------------------ | :----------------------------------------------------------------------------- |
| **Language**         | **Go (1.23+)**                                                                  | Single binary, type-safe, excellent concurrency for crawling.                  |
| **Web Framework**    | **[Gin](https://github.com/gin-gonic/gin)**                                     | The most popular Go web framework. Fast, easy middleware, huge community.      |
| **Static Scraper**   | **[Colly](https://github.com/gocolly/colly)**                                   | "Batteries included" scraper. Extremely fast for non-JS sites.                 |
| **Dynamic Scraper**  | **[Chromedp](https://github.com/chromedp/chromedp)**                            | Controls Chrome via DevTools Protocol. No Node.js/Puppeteer dependency needed. |
| **HTML -> Markdown** | **[html-to-markdown/v2](https://github.com/JohannesKaufmann/html-to-markdown)** | The standard for converting DOM to LLM-friendly text.                          |
| **Job Queue**        | **[Asynq](https://github.com/hibiken/asynq)**                                   | Redis-backed queue. Handles retries, scheduling, and reliability for us.       |
| **Database**         | **Redis**                                                                       | Used by Asynq. We will support **Leapcell/Upstash** (TLS/`rediss://`).         |
| **Config**           | **[Viper](https://github.com/spf13/viper)**                                     | Industry standard for handling env vars and config files.                      |
| **User Agents**      | **[gofakeit](https://github.com/brianvoe/gofakeit)**                            | Generates random, realistic User Agent strings for rotation.                   |

## 📅 Implementation Phases

### Phase 1: The Foundation (Static Scraping)

**Goal:** A working API that can scrape Wikipedia or simple blogs.

- Set up the **Standard Go Project Layout**.
- Initialize **Gin** server with structured logging.
- Implement the **Scraper Interface** with the **Colly** (static) engine.
- Create the `POST /v1/scrape` endpoint.
- Implement HTML-to-Markdown conversion.

### Phase 2: The "Heavy" Lifter (Dynamic Scraping)

**Goal:** Scrape React/Next.js apps that require JavaScript.

- Implement the **Chromedp** engine in the Scraper Interface.
- Add a "Smart Switch" or `render: true` flag to the API.
- Handle Docker complexity (installing Chromium inside the container).
- **Outcome:** Can scrape sites that return empty HTML without JS.

### Phase 3: Async Jobs & Queues

**Goal:** Crawl entire domains without timing out.

- Set up **Asynq** with Redis (handling TLS for Upstash/Leapcell).
- Create a **Worker** process that runs alongside the API.
- Implement `POST /v1/crawl` (enqueues job) and `GET /v1/crawl/:id` (checks status).
- Handle rate limiting and politeness (robots.txt).

### Phase 4: Production Hardening

**Goal:** Secure and deployable.

- **Auth:** Simple API Key middleware.
- **Config:** Robust `env` handling with Viper.
- **Docker:** Multi-stage build for small, secure images.
- **Docs:** Swagger/OpenAPI generation (optional but recommended).

## 📚 Key Resources

- [Performance & Reliability Plan](./performance_reliability/implementation_plan.md) - **New!** Optimization Strategy
- [Architecture & Code Samples](./architecture.md) - **Start Here for Code**
- [API Specification](./api-spec.md) - Request/Response formats
- [Environment Setup](./env.example) - Redis & Auth config
- [Actionable Todos](./todos.md) - Step-by-step tasks
