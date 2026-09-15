# 🔥 Tomoshibi: Go for Svelte Developers

A comprehensive guide to understanding this Go codebase, written specifically for full-stack Svelte/TypeScript developers.

> **Looking for the main index?** [Documentation Index](INDEX.md)
>
> **Looking for a practical workflow?** If you just want to know how to start the server, ping the API from SvelteKit, test, and debug, head over to the [Svelte Dev Workflow](SVELTE_DEV_WORKFLOW.md).

---

## Table of Contents

1. [Quick Mental Model](#quick-mental-model)
2. [Go vs TypeScript/JavaScript Cheatsheet](#go-vs-typescriptjavascript-cheatsheet)
3. [Go Syntax Reference](GO_SYNTAX_REFERENCE.md)
4. [Project Structure Explained](#project-structure-explained)
5. [The `go.mod` File (Like `package.json`)](#the-gomod-file-like-packagejson)
6. [Imports Deep Dive](#imports-deep-dive)
7. [Package System](#package-system)
8. [Types, Structs, and Interfaces](#types-structs-and-interfaces)
9. [Functions and Methods](#functions-and-methods)
10. [Error Handling](#error-handling)
11. [Pointers (The Scary Part That Isn't)](#pointers-the-scary-part-that-isnt)
12. [Context (Like SvelteKit's `event`)](#context-like-sveltekits-event)
13. [Concurrency with Goroutines](#concurrency-with-goroutines)
14. [File-by-File Breakdown](#file-by-file-breakdown)

---

## Quick Mental Model

| Svelte/JS Concept         | Go Equivalent          | Tomoshibi Example                                              |
| ------------------------- | ---------------------- | ----------------------------------------------------------- |
| `npm`/`pnpm`              | `go mod`               | `go.mod`                                                    |
| `package.json`            | `go.mod`               | Lists dependencies                                          |
| `import { x } from 'pkg'` | `import "pkg"`         | `import "github.com/gin-gonic/gin"`                         |
| `export function`         | Capitalized name       | `func NewService()` (exported) vs `func helper()` (private) |
| `interface`               | `interface`            | `type Scraper interface { ... }`                            |
| `class`                   | `struct` + methods     | `type Service struct { ... }`                               |
| `new Class()`             | Factory function       | `NewService()` returns `*Service`                           |
| SvelteKit route handlers  | Gin handlers           | `func (h *Handler) Scrape(c *gin.Context)`                  |
| `try/catch`               | Multiple return values | `result, err := fn()`                                       |
| `async/await`             | Goroutines + channels  | `go func() { ... }()`                                       |
| `null`/`undefined`        | `nil`                  | `if err != nil { ... }`                                     |

---

## Go vs TypeScript/JavaScript Cheatsheet

### Variable Declaration

```typescript
// TypeScript
const name: string = "tomoshibi";
let count: number = 0;
let items: string[] = ["a", "b"];
```

```go
// Go (from internal/config/config.go)
name := "tomoshibi"           // Short declaration (type inferred)
var count int = 0          // Explicit declaration
items := []string{"a", "b"} // Slice (dynamic array)
```

**Key differences:**

- `:=` declares AND assigns (only inside functions)
- `var` for package-level or explicit typing
- Type comes AFTER the variable name
- No semicolons needed (compiler adds them)

### Functions

```typescript
// TypeScript
function scrape(url: string, mode?: string): Promise<Result> {}
const scrape = (url: string) => {};
```

```go
// Go (from internal/scraper/service.go line 30)
func (s *Service) Scrape(ctx context.Context, url string, mode string) (*domain.ScrapeResult, error) {
    // ...
}
```

**Key differences:**

- `func` keyword required
- Return type at the END
- Can return MULTIPLE values (result + error)
- No arrow functions
- Optional params don't exist – use empty strings or pointers

### Anonymous Functions (Closures)

```typescript
// TypeScript
setTimeout(() => console.log("hello"), 1000);
```

```go
// Go (from cmd/api/main.go line 65)
go func() {
    if err := workerServer.Run(mux); err != nil {
        logger.Log.Error("Embedded Worker failed", "error", err)
    }
}()  // Note the () at the end - it calls the function immediately
```

---

## Project Structure Explained

```
tomoshibi/
├── cmd/                    # Entry points (like SvelteKit's +page.server.ts)
│   ├── api/main.go         # Main API server entry
│   └── worker/main.go      # Standalone worker entry
├── internal/               # Private packages (can't be imported externally)
│   ├── api/                # HTTP layer
│   │   ├── router.go       # Route definitions (like +server.ts files)
│   │   ├── handlers/       # Request handlers (like SvelteKit actions)
│   │   └── middleware/     # Middleware (like hooks.server.ts)
│   ├── config/             # Configuration loading
│   ├── domain/             # Core types/interfaces (like types.ts)
│   ├── scraper/            # Business logic for scraping
│   ├── search/             # Business logic for search
│   └── worker/             # Background job processing
├── pkg/                    # Public packages (reusable libraries)
│   └── logger/             # Logging utilities
└── go.mod                  # Dependencies (like package.json)
```

### Why `cmd/`, `internal/`, `pkg/`?

This is a **standard Go project layout**:

- **`cmd/`**: Executables. Each subfolder becomes a binary. Think of each as a separate `npm run dev` vs `npm run worker`.

- **`internal/`**: Private code. Go **enforces** that other projects can't import from `internal/`. This is like having all your code in a private `$lib` that nobody else can use.

- **`pkg/`**: Public libraries that OTHER projects could import. The `logger` here could theoretically be used by other Go projects.

---

## The `go.mod` File (Like `package.json`)

```go
// go.mod
module github.com/standard-user/tomoshibi  // Package name (like "name" in package.json)

go 1.25  // Go version required

require (
    github.com/gin-gonic/gin v1.11.0         // Web framework (like Express/Hono)
    github.com/hibiken/asynq v0.25.1         // Job queue (like BullMQ)
    github.com/redis/go-redis/v9 v9.17.2     // Redis client
    github.com/chromedp/chromedp v0.14.2     // Headless Chrome (like Puppeteer)
    github.com/gocolly/colly/v2 v2.3.0       // Fast scraper (like Cheerio)
    // ...
)
```

**Key commands:**

```bash
go mod download    # Like `npm install`
go mod tidy        # Removes unused deps, adds missing ones
go get pkg@v1.2.3  # Like `npm install pkg@1.2.3`
```

---

## Imports Deep Dive

### Import Syntax

```go
// internal/api/handlers/scrape.go
import (
    "net/http"  // Standard library (no domain = stdlib)

    "github.com/gin-gonic/gin"                      // External package
    "github.com/standard-user/tomoshibi/internal/scraper"  // Internal package
    "github.com/standard-user/tomoshibi/pkg/logger"       // Our logger package
)
```

**Import rules:**

1. Standard library has no domain prefix (`"fmt"`, `"net/http"`)
2. External packages use full GitHub/module path
3. Your own packages use your module name + path
4. Packages are FOLDERS, not files

### Aliased Imports

```go
// internal/scraper/colly.go line 8
import (
    md "github.com/JohannesKaufmann/html-to-markdown/v2"  // Alias as 'md'
)

// Usage:
markdown, err := md.ConvertString(htmlContent)
```

This is like `import * as md from 'html-to-markdown'` in TypeScript.

### Blank Import (Side Effects)

```go
import _ "some/package"  // Import for side effects only
```

Like `import 'some-css.css'` in JS – runs init code but doesn't use exports.

---

## Package System

### Every Folder is a Package

```go
// internal/domain/scraper.go
package domain  // Package declaration MUST be first line

// internal/scraper/service.go
package scraper  // Different folder = different package
```

**Key rule**: All `.go` files in the same folder MUST have the same `package` name.

### Exported vs Private (The Capitalization Rule)

```go
// Exported (PUBLIC) - Capitalized first letter
func NewService() *Service { }    // ✅ Can be used outside package
type ScrapeResult struct { }      // ✅ Can be used outside package
const MaxRetries = 3              // ✅ Can be used outside package

// Private - lowercase first letter
func helperFunction() { }         // ❌ Only usable within this package
type internalData struct { }      // ❌ Only usable within this package
```

This is Go's visibility system – **no `export` keyword needed**. Just capitalize!

### Using Packages

```go
// cmd/api/main.go
import "github.com/standard-user/tomoshibi/internal/scraper"

// You access exports via package name:
scraperService := scraper.NewService(...)  // package.ExportedFunc
```

---

## Types, Structs, and Interfaces

### Structs (Like TypeScript Classes/Types)

```typescript
// TypeScript
interface ScrapeResult {
  url: string;
  markdown: string;
  html?: string;
  metadata?: Record<string, string>;
}
```

```go
// internal/domain/scraper.go
type ScrapeResult struct {
    URL      string            `json:"url"`
    Markdown string            `json:"markdown"`
    HTML     string            `json:"html,omitempty"`
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

**Key differences:**

- Fields are capitalized (exported) or lowercase (private)
- Backtick tags (`` `json:"url"` ``) define JSON serialization
- `omitempty` = don't include if empty (like optional in TS)
- `map[string]string` = `Record<string, string>`

### Struct Tags Explained

```go
type ScrapeRequest struct {
    URL    string `json:"url" binding:"required,url"`
    Mode   string `json:"mode"`
}
```

- `json:"url"` – When serializing to JSON, use "url" as key
- `binding:"required,url"` – Gin validation: field is required and must be a URL
- Tags are metadata read by libraries at runtime

### Interfaces (Duck Typing!)

```go
// internal/domain/scraper.go
type Scraper interface {
    Scrape(ctx context.Context, url string) (*ScrapeResult, error)
}
```

**Go interfaces are implicit!** Any type that has a `Scrape` method matching this signature automatically implements `Scraper`. No `implements` keyword needed.

```go
// internal/scraper/colly.go
type CollyScraper struct{}

func (s *CollyScraper) Scrape(ctx context.Context, url string) (*domain.ScrapeResult, error) {
    // Implementation
}
// CollyScraper now implements Scraper automatically!

// internal/scraper/chromedp.go
type ChromedpScraper struct{ /* ... */ }

func (s *ChromedpScraper) Scrape(ctx context.Context, url string) (*domain.ScrapeResult, error) {
    // Different implementation
}
// ChromedpScraper also implements Scraper!
```

This is like TypeScript's structural typing but even more flexible.

---

## Functions and Methods

### Regular Functions

```go
// internal/config/config.go
func Load() (*Config, error) {
    // Returns pointer to Config AND an error
}
```

### Methods (Functions on Structs)

```go
// internal/scraper/service.go
type Service struct {
    colly    domain.Scraper
    chromedp domain.Scraper
    redis    *redis.Client
}

// Method with receiver - (s *Service) is like "this" in classes
func (s *Service) Scrape(ctx context.Context, url string, mode string) (*domain.ScrapeResult, error) {
    // s.colly, s.chromedp, s.redis are accessible
}
```

The `(s *Service)` part is called a **receiver**:

- `s` is like `this` in JavaScript/TypeScript
- `*Service` means it's a pointer receiver (can modify the struct)

### Constructor Pattern (Factory Functions)

Go doesn't have constructors. Instead, we use factory functions:

```go
// internal/scraper/service.go
func NewService(colly domain.Scraper, chromedp domain.Scraper, redis *redis.Client) *Service {
    return &Service{
        colly:    colly,
        chromedp: chromedp,
        redis:    redis,
    }
}
```

```go
// Usage in cmd/api/main.go
scraperService := scraper.NewService(collyScraper, chromedpScraper, redisClient)
```

The `New` prefix is a Go convention (like `createXxx` in JS/TS).

---

## Error Handling

### No try/catch – Multiple Return Values

```typescript
// TypeScript
try {
  const result = await scrape(url);
} catch (error) {
  console.error(error);
}
```

```go
// Go (from internal/api/handlers/scrape.go)
result, err := h.service.Scrape(c.Request.Context(), req.URL, mode)
if err != nil {
    logger.Log.Error("Scrape failed", "url", req.URL, "error", err)
    c.JSON(http.StatusInternalServerError, gin.H{"error": "Scraping failed"})
    return
}
// result is safe to use here
```

**Key patterns:**

- Functions return `(result, error)` tuple
- Always check `if err != nil` immediately
- `nil` is like `null` – means "no error"
- Handle errors early, return or panic

### Error Wrapping

```go
// internal/scraper/colly.go
if err != nil {
    return nil, fmt.Errorf("scraping failed: %w", err)
}
```

`%w` wraps the original error, preserving the chain (like `cause` in JS).

### The Blank Identifier `_`

```go
// internal/config/config.go
password, _ := u.User.Password()  // Ignore the boolean return value
```

Use `_` when you don't care about a return value.

---

## Pointers (The Scary Part That Isn't)

### Quick Explanation

```go
// Value - a copy
cfg := Config{Port: "8080"}

// Pointer - a reference to the original
cfg := &Config{Port: "8080"}  // & = "address of"
```

Think of pointers like JavaScript objects passed by reference vs primitive values.

### When to Use Pointers

```go
// internal/scraper/service.go
type Service struct {
    redis *redis.Client  // Pointer: might be nil (optional), shared resource
}

func NewService(...) *Service {  // Returns pointer
    return &Service{...}         // & creates a pointer to the struct
}
```

**Rules of thumb:**

- Use `*Type` (pointer) for:
  - Optional values that can be `nil`
  - Large structs (avoid copying)
  - When you need to modify the original
  - Resources like clients, connections
- Use `Type` (value) for:
  - Small, immutable data
  - When you want a copy

### Pointer Syntax Cheatsheet

```go
cfg := &Config{}     // Create pointer to new Config
*cfg                 // Dereference: get the value pointer points to
cfg.Port             // Shorthand: Go auto-dereferences for field access
var cfg *Config      // Declare nil pointer
if cfg != nil { }    // Check if pointer is not nil
```

---

## Context (Like SvelteKit's `event`)

`context.Context` carries:

- Request cancellation signals
- Timeouts
- Request-scoped values

```go
// internal/api/handlers/scrape.go
func (h *ScrapeHandler) Scrape(c *gin.Context) {
    result, err := h.service.Scrape(
        c.Request.Context(),  // Pass the HTTP request context
        req.URL,
        mode,
    )
}
```

```go
// internal/scraper/chromedp.go
func (s *ChromedpScraper) Scrape(ctx context.Context, url string) (*domain.ScrapeResult, error) {
    timeout := 60 * time.Second
    if dl, ok := ctx.Deadline(); ok {
        timeout = time.Until(dl)  // Respect parent timeout
    }
    taskCtx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
    defer cancelTimeout()  // Always cancel when done!
}
```

**Key patterns:**

- First parameter of most functions is `context.Context`
- Use `context.WithTimeout()` to add deadlines
- Call `defer cancel()` to clean up resources
- Check `ctx.Done()` for cancellation

---

## Concurrency with Goroutines

### Starting a Goroutine

```go
// cmd/api/main.go line 64-68
go func() {
    if err := workerServer.Run(mux); err != nil {
        logger.Log.Error("Embedded Worker failed", "error", err)
    }
}()
```

`go functionCall()` starts the function in a new lightweight thread.

### `defer` – Cleanup on Function Exit

```go
// internal/scraper/chromedp.go
taskCtx, cancelTask := chromedp.NewContext(s.allocCtx)
defer cancelTask()  // Will run when function returns, no matter what
```

`defer` is like `finally` but for single statements. Runs in LIFO order.

```go
// Multiple defers run in reverse order
defer fmt.Println("first")   // Runs third
defer fmt.Println("second")  // Runs second
defer fmt.Println("third")   // Runs first
```

---

## File-by-File Breakdown

### `cmd/api/main.go` – The API Entry Point

```go
package main  // Executable packages are named "main"

func main() {  // Entry point (like index.ts)
    // 1. Load configuration
    cfg, err := config.Load()

    // 2. Initialize logger
    logger.Init(cfg.App.LogLevel)

    // 3. Initialize services (dependency injection)
    collyScraper := scraper.NewCollyScraper()
    chromedpScraper := scraper.NewChromedpScraper()
    scraperService := scraper.NewService(collyScraper, chromedpScraper, redisClient)

    // 4. Start embedded worker in goroutine
    go func() {
        workerServer.Run(mux)
    }()

    // 5. Start HTTP server (blocking)
    router.Run(":8080")
}
```

### `internal/api/router.go` – Route Definitions

```go
func NewRouter(cfg *config.Config, logger *slog.Logger, ...) *gin.Engine {
    r := gin.New()  // Create router

    r.Use(gin.Recovery())           // Panic recovery middleware
    r.Use(middleware.Logger(logger)) // Custom logging middleware

    v1 := r.Group("/v1")  // Route group (like SvelteKit route groups)
    {
        v1.POST("/scrape", scrapeHandler.Scrape)
        v1.GET("/scrape", scrapeHandler.Scrape)
        v1.POST("/search", searchHandler.Search)
    }

    return r
}
```

### `internal/api/handlers/scrape.go` – Request Handler

```go
type ScrapeRequest struct {
    URL  string `json:"url" binding:"required,url"`  // Validation tags
    Mode string `json:"mode"`
}

type ScrapeHandler struct {
    service *scraper.Service  // Dependency injection
}

func NewScrapeHandler(s *scraper.Service) *ScrapeHandler {
    return &ScrapeHandler{service: s}
}

func (h *ScrapeHandler) Scrape(c *gin.Context) {
    var req ScrapeRequest

    // Bind JSON body to struct
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
        return
    }

    // Call service
    result, err := h.service.Scrape(c.Request.Context(), req.URL, mode)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Scraping failed"})
        return
    }

    // Return JSON response
    c.JSON(http.StatusOK, result)
}
```

### `internal/scraper/service.go` – Business Logic

```go
type Service struct {
    colly    domain.Scraper    // Interface type
    chromedp domain.Scraper    // Both implement same interface
    redis    *redis.Client     // Optional (can be nil)
}

func (s *Service) Scrape(ctx context.Context, url string, mode string) (*domain.ScrapeResult, error) {
    // 1. Try cache first
    if s.redis != nil {
        val, err := s.redis.Get(ctx, cacheKey).Result()
        if err == nil {
            // Return cached result
        }
    }

    // 2. Choose scraper based on mode
    switch mode {
    case "dynamic":
        result, err = s.chromedp.Scrape(ctx, url)
    case "static":
        result, err = s.colly.Scrape(ctx, url)
    case "smart":
        // Try static, fall back to dynamic if needed
    }

    // 3. Cache result
    if s.redis != nil {
        s.redis.Set(ctx, cacheKey, data, 7*24*time.Hour)
    }

    return result, nil
}
```

### `internal/domain/scraper.go` – Core Types

```go
package domain

import "context"

// Data structure
type ScrapeResult struct {
    URL      string            `json:"url"`
    Markdown string            `json:"markdown"`
    HTML     string            `json:"html,omitempty"`
    Metadata map[string]string `json:"metadata,omitempty"`
}

// Interface - defines contract
type Scraper interface {
    Scrape(ctx context.Context, url string) (*ScrapeResult, error)
}
```

### `pkg/logger/logger.go` – Shared Utility

```go
package logger

import (
    "log/slog"  // Go 1.21+ structured logger
    "os"
)

var Log *slog.Logger  // Package-level variable

func Init(level string) {
    var logLevel slog.Level
    switch level {
    case "debug":
        logLevel = slog.LevelDebug
    // ...
    }

    handler := slog.NewJSONHandler(os.Stdout, opts)
    Log = slog.New(handler)
}
```

Usage elsewhere:

```go
logger.Log.Info("Starting server", "port", cfg.Server.Port)
```

---

## Common Patterns in This Codebase

### 1. Dependency Injection via Constructors

```go
// Create dependencies
colly := scraper.NewCollyScraper()
chromedp := scraper.NewChromedpScraper()

// Inject into service
service := scraper.NewService(colly, chromedp, redisClient)

// Inject into handler
handler := handlers.NewScrapeHandler(service)

// Inject into router
router := api.NewRouter(cfg, logger, handler, crawlHandler, searchHandler)
```

### 2. Interface-Based Design

```go
// domain/scraper.go defines interface
type Scraper interface {
    Scrape(ctx context.Context, url string) (*ScrapeResult, error)
}

// Two implementations
type CollyScraper struct{}      // Fast, static
type ChromedpScraper struct{}   // Slow, dynamic

// Service accepts interface, not concrete type
type Service struct {
    colly    domain.Scraper  // Could be any implementation
    chromedp domain.Scraper
}
```

### 3. Graceful Shutdown

```go
chromedpScraper := scraper.NewChromedpScraper()
defer chromedpScraper.Close()  // Clean up browser on exit
```

### 4. Config via Environment + Viper

```go
// internal/config/config.go
v := viper.New()
v.AutomaticEnv()  // Read from environment
v.SetDefault("server.port", "8080")  // Defaults
v.Unmarshal(&cfg)  // Parse into struct
```

---

## Quick Reference: Go Syntax You'll See Often

```go
// Short variable declaration
x := 5

// If with initialization
if err := doSomething(); err != nil {
    return err
}

// Type switch
switch v := x.(type) {
case string:
    // v is string
case int:
    // v is int
}

// String formatting
fmt.Sprintf("Hello %s, you have %d messages", name, count)
fmt.Errorf("failed to do X: %w", err)  // %w wraps error

// Slice operations
items := []string{"a", "b", "c"}
items = append(items, "d")
first := items[0]
rest := items[1:]

// Map operations
m := map[string]int{"a": 1, "b": 2}
m["c"] = 3
val, exists := m["key"]  // Check if key exists

// Range loop
for i, item := range items {
    fmt.Println(i, item)
}
for key, value := range m {
    fmt.Println(key, value)
}

// Goroutine
go doSomethingAsync()

// Channel (communication between goroutines)
ch := make(chan string)
ch <- "message"  // Send
msg := <-ch      // Receive
```

---

## Next Steps

1. **Read the handlers** in `internal/api/handlers/` – they're the closest to SvelteKit route handlers
2. **Trace a request** from `main.go` → `router.go` → `handlers/*.go` → `service.go`
3. **Experiment** with `go run ./cmd/api` and hit the endpoints
4. **Check the tests** in `*_test.go` files for usage examples

Welcome to Go! 🎉
