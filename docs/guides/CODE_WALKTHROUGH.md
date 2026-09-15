# 📂 Tomoshibi Codebase Walkthrough

A detailed file-by-file exploration of the Tomoshibi codebase with annotated code examples.

> [!TIP]
> This is a deep dive. For a high-level overview, check the [Architecture Guide](ARCHITECTURE.md) or the [Project Tour](PROJECT_TOUR.md).
> See [Documentation Index](INDEX.md) for related guides.

---

## Entry Points (`cmd/`)

### `cmd/api/main.go` – Main Server

This is the application's entry point. Let's break it down:

```go
package main  // ← "main" package = executable binary
```

**Import groups** – Go convention groups imports: stdlib, external, internal:

```go
import (
    // Standard library
    "fmt"
    "os"

    // Our internal packages
    "github.com/standard-user/tomoshibi/internal/api"
    "github.com/standard-user/tomoshibi/internal/api/handlers"
    "github.com/standard-user/tomoshibi/internal/config"
    "github.com/standard-user/tomoshibi/internal/scraper"
    "github.com/standard-user/tomoshibi/internal/search"
    "github.com/standard-user/tomoshibi/internal/worker"
    "github.com/standard-user/tomoshibi/pkg/logger"

    // External packages
    "github.com/hibiken/asynq"
    "github.com/redis/go-redis/v9"
)
```

**The `main()` function** – Application lifecycle:

```go
func main() {
    // 1. Load Config
    cfg, err := config.Load()
    if err != nil {
        fmt.Printf("Failed to load config: %v\n", err)
        os.Exit(1)  // Exit with error code
    }
```

**Dependency injection pattern** – Creating services and wiring them together:

```go
    // Create scrapers (both implement domain.Scraper interface)
    collyScraper := scraper.NewCollyScraper()
    chromedpScraper := scraper.NewChromedpScraper()
    defer chromedpScraper.Close()  // ← cleanup when main() exits

    // Wire them into service
    scraperService := scraper.NewService(collyScraper, chromedpScraper, redisClient)

    // Wire service into handler
    scrapeHandler := handlers.NewScrapeHandler(scraperService)
```

**Monolith mode** – Starting worker in a goroutine:

```go
    // go func() { } starts a concurrent goroutine
    go func() {
        if err := workerServer.Run(mux); err != nil {
            logger.Log.Error("Embedded Worker failed", "error", err)
        }
    }()  // ← () calls the anonymous function immediately
```

This is like JavaScript's:

```javascript
// JavaScript equivalent
(async () => {
  await workerServer.run(mux);
})();
```

---

### `cmd/worker/main.go` – Standalone Worker

Nearly identical to `cmd/api/main.go` but **only runs the worker**, no HTTP server.

Key difference:

```go
// This blocks and processes jobs forever
if err := srv.Run(mux); err != nil {
    logger.Log.Error("Could not run worker server", "error", err)
    os.Exit(1)
}
```

---

## Configuration (`internal/config/`)

### `internal/config/config.go`

**Struct tags for mapping** – Notice the backtick annotations:

```go
type Config struct {
    Server ServerConfig `mapstructure:"server"`  // ← Viper uses "mapstructure" tags
    App    AppConfig    `mapstructure:"app"`
    Redis  RedisConfig  `mapstructure:"redis"`
    Brave  BraveConfig  `mapstructure:"brave"`
}

type ServerConfig struct {
    Port string `mapstructure:"port"`
    Mode string `mapstructure:"mode"`  // debug, release, test
}
```

**Environment variable binding**:

```go
func Load() (*Config, error) {
    // Load .env file (fails silently if missing)
    godotenv.Load()

    v := viper.New()
    v.AutomaticEnv()  // Read from environment variables
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))  // SERVER_PORT → server.port

    // Set defaults
    v.SetDefault("server.port", "8080")
    v.SetDefault("server.mode", "debug")

    // Custom binding for non-standard env var names
    v.BindEnv("brave.api_key", "BRAVE_SEARCH_API_KEY")

    // Unmarshal into struct
    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, err  // ← Return nil pointer and error
    }

    return &cfg, nil  // ← Return pointer to cfg, no error
}
```

**String building with `fmt.Sprintf`**:

```go
// Build Redis URL from parts
cfg.Redis.URL = fmt.Sprintf("redis://:%s@%s", cfg.Redis.Password, addr)
//                          ↑ format string    ↑ arguments fill in %s
```

---

## HTTP Layer (`internal/api/`)

### `internal/api/router.go`

**Function signature with multiple parameters of same type**:

```go
func NewRouter(
    cfg *config.Config,              // pointer to Config
    logger *slog.Logger,             // pointer to Logger
    scrapeHandler *handlers.ScrapeHandler,
    crawlHandler *handlers.CrawlHandler,  // can be nil!
    searchHandler *handlers.SearchHandler,
) *gin.Engine {  // ← returns pointer to gin.Engine
```

**Gin router setup**:

```go
    if cfg.Server.Mode == "release" {
        gin.SetMode(gin.ReleaseMode)
    }

    r := gin.New()                      // Create router
    r.Use(gin.Recovery())               // Middleware: catch panics
    r.Use(middleware.Logger(logger))    // Middleware: custom logging
```

**Route groups** – Like SvelteKit route groups:

```go
    v1 := r.Group("/v1")   // All routes prefixed with /v1
    {
        v1.POST("/scrape", scrapeHandler.Scrape)
        v1.GET("/scrape", scrapeHandler.Scrape)   // Same handler, both methods
        v1.POST("/search", searchHandler.Search)
    }
```

**Conditional routes based on nil check**:

```go
    if crawlHandler != nil {
        v1.POST("/crawl", crawlHandler.EnqueueCrawl)
        v1.GET("/crawl/:id", crawlHandler.GetCrawlStatus)  // :id = path parameter
    } else {
        // Return 503 when Redis not available
        v1.POST("/crawl", func(c *gin.Context) {
            c.JSON(http.StatusServiceUnavailable, gin.H{
                "error": "Asynchronous crawling is not available",
            })
        })
    }
```

`gin.H{}` is shorthand for `map[string]interface{}` – a map that can hold any value type.

---

### `internal/api/handlers/scrape.go`

**Request struct with validation tags**:

```go
type ScrapeRequest struct {
    URL    string `json:"url" binding:"required,url"`  // required AND must be valid URL
    Render bool   `json:"render"`                      // optional, defaults to false
    Mode   string `json:"mode"`                        // "smart", "static", "dynamic"
}
```

**Handler struct** – Holds dependencies:

```go
type ScrapeHandler struct {
    service *scraper.Service  // injected dependency
}

// Constructor function
func NewScrapeHandler(s *scraper.Service) *ScrapeHandler {
    return &ScrapeHandler{service: s}  // & = address of (creates pointer)
}
```

**Handler method** – The receiver `(h *ScrapeHandler)`:

```go
func (h *ScrapeHandler) Scrape(c *gin.Context) {
//   ↑ receiver: h is "this", *ScrapeHandler is the type
//   This makes Scrape a METHOD on ScrapeHandler
```

**JSON binding with error handling**:

```go
    var req ScrapeRequest  // Declare empty struct

    // Try to bind JSON body
    if c.Request.Method == http.MethodPost && c.Request.ContentLength > 0 {
        if err := c.ShouldBindJSON(&req); err != nil {
            //                      ↑ pass address of req (so Gin can fill it)
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
            return  // ← early return (no else needed)
        }
    }
```

**Query parameter parsing**:

```go
    // Override with query params (for GET requests)
    if url := c.Query("url"); url != "" {
        req.URL = url
    }
    // ↑ if-with-init: url is scoped to this if block only
```

**Calling service and returning response**:

```go
    result, err := h.service.Scrape(c.Request.Context(), req.URL, mode)
    if err != nil {
        logger.Log.Error("Scrape failed", "url", req.URL, "error", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Scraping failed"})
        return
    }

    c.JSON(http.StatusOK, result)  // result is automatically serialized to JSON
```

---

### `internal/api/handlers/crawl.go`

**Asynq client setup** with TLS handling:

```go
func NewCrawlHandler(redisAddr string) (*CrawlHandler, error) {
    u, err := url.Parse(redisAddr)
    if err != nil {
        return nil, fmt.Errorf("failed to parse redis url: %w", err)
        //                                                   ↑ %w wraps original error
    }

    password, _ := u.User.Password()  // _ ignores the bool return value
    //          ↑ blank identifier

    redisOpt := asynq.RedisClientOpt{
        Addr:     addr,
        Password: password,
    }

    // Handle rediss:// (TLS) URLs
    if u.Scheme == "rediss" {
        redisOpt.TLSConfig = &tls.Config{
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
        }
    }

    return &CrawlHandler{
        client:    asynq.NewClient(redisOpt),
        inspector: asynq.NewInspector(redisOpt),
    }, nil
}
```

**Close method for cleanup**:

```go
func (h *CrawlHandler) Close() {
    h.client.Close()
    h.inspector.Close()
}
// Usage: defer crawlHandler.Close()
```

**Enqueue job and return ID**:

```go
func (h *CrawlHandler) EnqueueCrawl(c *gin.Context) {
    var req CrawlRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Create task payload (screenshot, images, imageFormat are optional params)
    task, err := worker.NewScrapeTask(req.URL, req.Render, false, false, "")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
        return
    }

    // Enqueue to Redis
    info, err := h.client.Enqueue(task)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue task"})
        return
    }

    // Return 202 Accepted with job ID
    c.JSON(http.StatusAccepted, CrawlResponse{
        ID:     info.ID,
        URL:    req.URL,
        Render: req.Render,
    })
}
```

---

### `internal/api/middleware/logger.go`

**Middleware function** – Returns a `gin.HandlerFunc`:

```go
func Logger(logger *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {  // ← returns this closure
        start := time.Now()

        c.Next()  // ← call next handler in chain

        latency := time.Since(start)

        logger.Info("Request",
            "method", c.Request.Method,  // key-value pairs for structured logging
            "path", c.Request.URL.Path,
            "status", c.Writer.Status(),
            "latency", latency,
            "ip", c.ClientIP(),
        )
    }
}
```

This is like Express middleware:

```javascript
// Express equivalent
function logger(logger) {
  return (req, res, next) => {
    const start = Date.now();
    next();
    logger.info({ path: req.path, latency: Date.now() - start });
  };
}
```

---

## Domain Types (`internal/domain/`)

### `internal/domain/scraper.go`

**Core data structure**:

```go
type ScrapeResult struct {
    URL      string            `json:"url"`
    Markdown string            `json:"markdown"`
    HTML     string            `json:"html,omitempty"`      // omitempty = exclude if empty
    Metadata map[string]string `json:"metadata,omitempty"`  // map = JS object/Record
}
```

**Interface definition**:

```go
type Scraper interface {
    Scrape(ctx context.Context, url string) (*ScrapeResult, error)
}
```

Any type with a matching `Scrape` method automatically implements this interface.

---

## Scraper Implementations (`internal/scraper/`)

### `internal/scraper/service.go`

**Service orchestrating multiple scrapers**:

```go
type Service struct {
    colly    domain.Scraper  // ← interface type (can be any Scraper)
    chromedp domain.Scraper
    redis    *redis.Client   // ← pointer (can be nil)
}

func NewService(colly domain.Scraper, chromedp domain.Scraper, redis *redis.Client) *Service {
    return &Service{
        colly:    colly,
        chromedp: chromedp,
        redis:    redis,
    }
}
```

**Caching with compression**:

```go
func (s *Service) Scrape(ctx context.Context, url string, mode string) (*domain.ScrapeResult, error) {
    cacheKey := fmt.Sprintf("scrape:%s:%s", url, mode)

    // Try cache if Redis is configured
    if s.redis != nil {
        val, err := s.redis.Get(ctx, cacheKey).Result()
        if err == nil {  // cache hit
            // Decompress
            b := bytes.NewReader([]byte(val))
            gz, err := gzip.NewReader(b)
            if err == nil {
                defer gz.Close()  // ← cleanup when function exits
                decompressed, _ := io.ReadAll(gz)

                var result domain.ScrapeResult
                json.Unmarshal(decompressed, &result)
                result.Metadata["cached"] = "true"
                return &result, nil
            }
        }
    }
```

**Switch statement for mode selection**:

```go
    switch mode {
    case "dynamic":
        result, err = s.chromedp.Scrape(ctx, url)  // ← polymorphism!
    case "static":
        result, err = s.colly.Scrape(ctx, url)
    case "smart":
        // Try static first
        result, err = s.colly.Scrape(ctx, url)
        if result != nil && ShouldUseDynamic(result.HTML) {
            result, err = s.chromedp.Scrape(ctx, url)
        }
    default:
        return nil, fmt.Errorf("unknown mode: %s", mode)
    }
```

**Helper functions defined inline**:

```go
    // Define helpers inside function as closures
    runDynamic := func() (*domain.ScrapeResult, error) {
        if s.chromedp == nil {
            return nil, fmt.Errorf("dynamic scraper not configured")
        }
        return s.chromedp.Scrape(ctx, url)
    }
```

---

### `internal/scraper/colly.go` – Fast Static Scraper

**Colly collector setup**:

```go
func (s *CollyScraper) Scrape(ctx context.Context, url string) (*domain.ScrapeResult, error) {
    c := colly.NewCollector(
        colly.Async(true),  // ← functional options pattern
    )

    // Event handler: on each request
    c.OnRequest(func(r *colly.Request) {
        r.Headers.Set("User-Agent", gofakeit.UserAgent())  // Random user agent
    })

    c.SetRequestTimeout(30 * time.Second)
```

**Closure capturing variables**:

```go
    var htmlContent string
    var scrapeErr error

    // Callback captures htmlContent from outer scope
    c.OnHTML("html", func(e *colly.HTMLElement) {
        htmlContent, _ = e.DOM.Html()  // Modifies outer variable
    })

    c.OnError(func(r *colly.Response, err error) {
        scrapeErr = fmt.Errorf("scraping failed: %w", err)
    })
```

**Async wait**:

```go
    err := c.Visit(url)
    if err != nil {
        return nil, err
    }

    c.Wait()  // Block until all async operations complete
```

**HTML to Markdown conversion**:

```go
    markdown, err := md.ConvertString(htmlContent)
    if err != nil {
        return nil, fmt.Errorf("markdown conversion failed: %w", err)
    }
```

**Building result struct**:

```go
    return &domain.ScrapeResult{
        URL:      url,
        Markdown: markdown,
        HTML:     htmlContent,
        Metadata: map[string]string{
            "scraped_at": time.Now().Format(time.RFC3339),
            "engine":     "colly",
        },
    }, nil
```

---

### `internal/scraper/chromedp.go` – Headless Browser Scraper

**Browser pool with shared context**:

```go
type ChromedpScraper struct {
    allocCtx context.Context  // Shared browser allocator
    cancel   context.CancelFunc
}

func NewChromedpScraper() *ChromedpScraper {
    opts := append(chromedp.DefaultExecAllocatorOptions[:],  // Spread defaults
        chromedp.Flag("headless", true),
        chromedp.Flag("disable-gpu", true),
        chromedp.Flag("no-sandbox", true),
        chromedp.Flag("disable-dev-shm-usage", true),  // Docker fix
    )

    allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
```

**Prewarming the browser**:

```go
    // Start browser immediately in background
    go func() {
        ctx, c := chromedp.NewContext(allocCtx)
        defer c()
        chromedp.Run(ctx)  // Warm up
    }()
```

**Scraping with timeout**:

```go
func (s *ChromedpScraper) Scrape(ctx context.Context, url string) (*domain.ScrapeResult, error) {
    // Create new tab (context) from shared browser
    taskCtx, cancelTask := chromedp.NewContext(s.allocCtx)
    defer cancelTask()

    // Respect parent timeout
    timeout := 60 * time.Second
    if dl, ok := ctx.Deadline(); ok {
        timeout = time.Until(dl)
    }
    taskCtx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
    defer cancelTimeout()

    var htmlContent string

    // Chromedp action chain
    err := chromedp.Run(taskCtx,
        chromedp.Navigate(url),
        chromedp.WaitVisible("body", chromedp.ByQuery),
        chromedp.OuterHTML("html", &htmlContent),
    )
```

---

### `internal/scraper/heuristics.go` – Smart Mode Logic

**Detecting SPAs and JS-required pages**:

```go
func ShouldUseDynamic(htmlBody string) bool {
    lowerBody := strings.ToLower(htmlBody)

    // Check for noscript warnings
    if strings.Contains(lowerBody, "<noscript>") {
        if strings.Contains(lowerBody, "enable javascript") ||
           strings.Contains(lowerBody, "need javascript") {
            return true
        }
    }

    // Check for SPA framework markers
    spaRoots := []string{
        `id="root"`,       // React
        `id="__next"`,     // Next.js
        `__NEXT_DATA__`,
        `ng-version`,      // Angular
    }

    for _, marker := range spaRoots {
        if strings.Contains(htmlBody, marker) {
            if len(htmlBody) < 5000 {  // Small = likely just shell
                return true
            }
        }
    }

    // Tiny body with scripts = SPA shell
    if len(htmlBody) < 2000 && strings.Contains(lowerBody, "<script") {
        return true
    }

    return false
}
```

---

## Worker System (`internal/worker/`)

### `internal/worker/tasks.go` – Task Definitions

**Payload struct**:

```go
type ScrapePayload struct {
    URL    string `json:"url"`
    Render bool   `json:"render"`
    Mode   string `json:"mode"`
}
```

**Task factory with options**:

```go
const TypeScrape = "scrape:url"  // Task type identifier

func NewScrapeTask(url string, render bool, screenshot bool, images bool, imageFormat string) (*asynq.Task, error) {
    payload := ScrapePayload{
        URL:         url,
        Render:      render,
        Screenshot:  screenshot,
        Images:      images,
        ImageFormat: imageFormat,
    }

    data, err := json.Marshal(payload)  // Serialize to JSON bytes
    if err != nil {
        return nil, fmt.Errorf("failed to marshal payload: %w", err)
    }

    return asynq.NewTask(
        TypeScrape,
        data,
        asynq.Retention(7*24*time.Hour),  // Keep result for 7 days
    ), nil
}
```

---

### `internal/worker/handlers.go` – Task Processing

**Task handler struct**:

```go
type ScrapeTaskHandler struct {
    scraper *scraper.Service
    logger  *slog.Logger
}

func NewScrapeTaskHandler(scraper *scraper.Service, logger *slog.Logger) *ScrapeTaskHandler {
    return &ScrapeTaskHandler{scraper: scraper, logger: logger}
}
```

**ProcessTask method**:

```go
func (h *ScrapeTaskHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
    // Unmarshal payload
    var payload ScrapePayload
    if err := json.Unmarshal(t.Payload(), &payload); err != nil {
        return fmt.Errorf("failed to unmarshal: %w", err)
    }

    h.logger.Info("Processing task",
        "url", payload.URL,
        "task_id", t.ResultWriter().TaskID(),
    )

    // Do the actual work
    result, err := h.scraper.Scrape(ctx, payload.URL, mode)
    if err != nil {
        return fmt.Errorf("scraping failed: %w", err)
    }

    // Write result (viewable via /v1/crawl/:id)
    t.ResultWriter().Write([]byte(fmt.Sprintf("Scraped %s successfully", payload.URL)))

    return nil  // nil = success
}
```

---

### `internal/worker/server.go` – Asynq Server

**Custom logger adapter**:

```go
type AsynqLogger struct {
    logger *slog.Logger
}

func (l *AsynqLogger) Debug(args ...interface{}) {
    l.logger.Debug(fmt.Sprint(args...))  // fmt.Sprint joins args
}
func (l *AsynqLogger) Info(args ...interface{}) {
    l.logger.Info(fmt.Sprint(args...))
}
// ... Error, Warn, Fatal
```

**Server configuration**:

```go
func NewServer(cfg *config.Config, logger *slog.Logger) *asynq.Server {
    srv := asynq.NewServer(
        redisOpt,
        asynq.Config{
            Concurrency: 10,  // Process 10 jobs simultaneously
            Queues: map[string]int{
                "critical": 6,  // 60% weight
                "default":  3,  // 30% weight
                "low":      1,  // 10% weight
            },
            TaskCheckInterval: 1 * time.Second,
            Logger:            &AsynqLogger{logger: logger},
        },
    )
    return srv
}
```

**Registering handlers**:

```go
func RegisterHandlers(mux *asynq.ServeMux, scraper *scraper.Service, logger *slog.Logger) {
    handler := NewScrapeTaskHandler(scraper, logger)
    mux.HandleFunc(TypeScrape, handler.ProcessTask)
    //            ↑ task type   ↑ handler function
}
```

---

## Search Service (`internal/search/`)

### `internal/search/service.go`

**Interface definition**:

```go
type Service interface {
    Search(ctx context.Context, opts SearchOptions) ([]Result, int, error)
}
```

**Rate limiter setup**:

```go
type BraveService struct {
    apiKey  string
    client  *http.Client
    limiter *rate.Limiter
}

func NewBraveService(apiKey string) *BraveService {
    return &BraveService{
        apiKey: apiKey,
        client: &http.Client{Timeout: 30 * time.Second},
        limiter: rate.NewLimiter(
            rate.Every(1100*time.Millisecond),  // 1 request per 1.1 seconds
            1,                                   // burst of 1
        ),
    }
}
```

**HTTP request with context**:

```go
func (s *BraveService) Search(ctx context.Context, opts SearchOptions) ([]Result, int, error) {
    // Wait for rate limiter
    if err := s.limiter.Wait(ctx); err != nil {
        return nil, 0, fmt.Errorf("rate limit wait: %w", err)
    }

    // Create request with context (for cancellation)
    req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to create request: %w", err)
    }

    // Add query parameters
    q := req.URL.Query()
    q.Add("q", opts.Query)
    q.Add("count", fmt.Sprintf("%d", opts.Limit))
    req.URL.RawQuery = q.Encode()

    // Set headers
    req.Header.Set("Accept", "application/json")
    req.Header.Set("X-Subscription-Token", s.apiKey)

    // Execute
    resp, err := s.client.Do(req)
    if err != nil {
        return nil, 0, err
    }
    defer resp.Body.Close()  // ← Always close response body!
```

**JSON decoding**:

```go
    var braveResponse BraveSearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&braveResponse); err != nil {
        return nil, 0, fmt.Errorf("failed to decode response: %w", err)
    }
```

---

## Logger Package (`pkg/logger/`)

### `pkg/logger/logger.go`

**Package-level variable**:

```go
package logger

var Log *slog.Logger  // ← Global logger, accessible as logger.Log
```

**Initialization function**:

```go
func Init(level string) {
    var logLevel slog.Level
    switch level {
    case "debug":
        logLevel = slog.LevelDebug
    case "info":
        logLevel = slog.LevelInfo
    case "warn":
        logLevel = slog.LevelWarn
    case "error":
        logLevel = slog.LevelError
    default:
        logLevel = slog.LevelInfo
    }

    opts := &slog.HandlerOptions{Level: logLevel}
    handler := slog.NewJSONHandler(os.Stdout, opts)
    Log = slog.New(handler)
    slog.SetDefault(Log)  // Also set as default logger
}
```

**Usage elsewhere**:

```go
import "github.com/standard-user/tomoshibi/pkg/logger"

logger.Log.Info("Starting server", "port", 8080)
logger.Log.Error("Something failed", "error", err)
```

---

## Key Patterns Summary

| Pattern          | Example                    | Purpose                      |
| ---------------- | -------------------------- | ---------------------------- |
| Factory function | `NewService()`             | Construct and return pointer |
| Receiver method  | `(s *Service) Scrape()`    | Method on struct             |
| Interface        | `type Scraper interface{}` | Define contracts             |
| Error handling   | `if err != nil { return }` | Early returns                |
| Defer            | `defer file.Close()`       | Cleanup on exit              |
| Goroutine        | `go func() {}()`           | Concurrent execution         |
| Context          | `ctx context.Context`      | Cancellation/timeout         |
| Struct tags      | `` `json:"url"` ``         | Serialization hints          |
| Blank identifier | `val, _ := fn()`           | Ignore return value          |

---

## Building and Running

```bash
# Run the API (development)
go run ./cmd/api

# Run standalone worker
go run ./cmd/worker

# Build binaries
go build -o bin/api ./cmd/api
go build -o bin/worker ./cmd/worker

# Run built binary
./bin/api

# Run tests
go test ./...
```
