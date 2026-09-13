package main

// @title           Cinder API
// @version         1.0
// @description     Web scraping, crawling, and AI data extraction API.
// @BasePath        /v1
import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/Michael-Obele/tomoshibi/internal/api/docs" // swagger docs side-effect; keep at app root

	"github.com/Michael-Obele/tomoshibi/internal/api"
	"github.com/Michael-Obele/tomoshibi/internal/api/handlers"
	"github.com/Michael-Obele/tomoshibi/internal/config"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/Michael-Obele/tomoshibi/internal/search"
	"github.com/Michael-Obele/tomoshibi/internal/worker"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// mustRedisOpt builds the asynq Redis option for the monitor scheduler's
// enqueuer, panicking only when the URL is unparseable (already validated).
func mustRedisOpt(cfg *config.Config) asynq.RedisClientOpt {
	opt, err := worker.RedisClientOpt(cfg.Redis.URL)
	if err != nil {
		panic(err)
	}
	return opt
}

// shutdownTimeout bounds graceful shutdown (env SHUTDOWN_TIMEOUT seconds,
// default 20s).
//
// Keep this below the platform's kill grace period or the process is
// SIGKILLed mid-drain, which defeats the point. On Fly.io that is
// kill_timeout in fly.toml.
func shutdownTimeout() time.Duration {
	if raw := os.Getenv("SHUTDOWN_TIMEOUT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 20 * time.Second
}

// workerDrainGrace bounds how long we wait for the embedded worker after the
// HTTP server has drained.
//
// It is deliberately much shorter than shutdownTimeout, because the two are
// not equally urgent. An in-flight HTTP response is lost forever if we drop
// it, so it gets the full budget. A queued task is durable: asynq re-queues
// what it aborts, and anything still marked active is recovered on the next
// start once its lease expires. Waiting longer costs deploy latency and buys
// a retry we already get for free.
//
// The wait is short specifically because it cannot be made reliable: asynq's
// processor sleeps up to TaskCheckInterval uninterruptibly when queues are
// empty and Shutdown blocks behind that sleep, so with our 15s interval even
// an idle worker can take 22.5s to acknowledge. Blocking the process on that
// stalls every deploy and risks a platform SIGKILL mid-drain.
const workerDrainGrace = 5 * time.Second

func main() {
	if err := run(); err != nil {
		// logger may be nil if we failed before Init.
		if logger.Log != nil {
			logger.Log.Error("Fatal error", "error", err)
		} else {
			fmt.Printf("Fatal error: %v\n", err)
		}
		os.Exit(1)
	}
}

// run wires up the process and blocks until a shutdown signal arrives.
// It is separate from main so deferred cleanup actually executes: os.Exit
// in main would skip every defer, which is how the browser used to leak.
func run() error {
	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Init Logger
	logger.Init(cfg.App.LogLevel)
	logger.Log.Info("Starting Cinder API", "port", cfg.Server.Port, "mode", cfg.Server.Mode)

	// Signal context: SIGTERM is routine, not exceptional. Fly.io's
	// auto_stop sends it whenever the machine idles down, so this path runs
	// constantly in normal operation rather than only on deploys.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Auto-generate Swagger Docs in debug mode
	if cfg.Server.Mode == "debug" {
		logger.Log.Info("Auto-generating Swagger docs...")
		cmd := exec.Command("go", "run", "github.com/swaggo/swag/cmd/swag@latest", "init", "-d", "./cmd/api,./internal/api/handlers,./internal/domain", "-g", "main.go", "-o", "internal/api/docs", "--parseDependency", "--parseInternal")
		// Assuming we're running from the root of the project
		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Log.Error("Failed to auto-generate swagger docs", "error", err, "output", string(output))
		} else {
			logger.Log.Info("Swagger docs auto-generated successfully")
		}
	}

	// 3. Init Scraper
	var redisClient *redis.Client
	if cfg.Redis.URL != "" {
		opt, err := redis.ParseURL(cfg.Redis.URL)
		if err != nil {
			logger.Log.Warn("Invalid Redis URL", "error", err)
		} else {
			redisClient = redis.NewClient(opt)
			defer redisClient.Close()
			logger.Log.Info("Redis caching enabled")
		}
	}

	collyScraper := scraper.NewCollyScraper()
	chromedpScraper := scraper.NewChromedpScraperWithLimit(cfg.App.ChromeRecycleAfter)
	defer chromedpScraper.Close()
	scraperService := scraper.NewService(collyScraper, chromedpScraper, redisClient)

	// Initialize Handlers
	scrapeHandler := handlers.NewScrapeHandler(scraperService)

	// Search: self-hosted SearXNG (primary, free, aggregates many engines)
	// with Brave as an optional fallback when a key is set. Results are
	// cached in Redis (when available) so repeat queries don't hammer the
	// upstream engine.
	// Stealth fallback: when STEALTH_ENABLED=true (via Viper
	// search.stealth_enabled), reuse the same ChromedpScraper allocator that
	// the scraper service uses — no second browser, respects recycleAfter
	// and tini. Disabled by default.
	var stealthFetcher search.BrowserFetcher
	if cfg.Search.StealthEnabled {
		stealthFetcher = chromedpScraper
		logger.Log.Info("Stealth search enabled (reusing chromedp allocator)")
	}
	searchSvc := search.NewHybridServiceWithStealth(cfg.Brave.APIKey, cfg.Search.SearXNGEndpoint, stealthFetcher)
	searchSvc = search.NewCachedService(searchSvc, redisClient)
	searchHandler := handlers.NewSearchHandler(searchSvc)

	// Try to initialize crawl handler (requires Redis)
	var crawlHandler *handlers.CrawlHandler
	var workerServer *asynq.Server
	if cfg.Redis.URL != "" {
		handler, err := handlers.NewCrawlHandler(cfg.Redis.URL)
		if err != nil {
			logger.Log.Warn("Redis not available, asynchronous crawling disabled", "error", err)
		} else {
			crawlHandler = handler
			defer crawlHandler.Close()
			logger.Log.Info("Asynchronous crawling enabled with Redis")

			// Monolith Mode: Start Embedded Worker if enabled (defaulting to TRUE for Hobby Tier)
			if os.Getenv("DISABLE_WORKER") != "true" {
				logger.Log.Info("Starting Embedded Worker (Monolith Mode)")
				workerServer = worker.NewServer(cfg, logger.Log)
				mux := asynq.NewServeMux()
				worker.RegisterHandlers(mux, scraperService, logger.Log)

				// Change-tracking monitors: register handler + scheduler.
				monitorKV := worker.NewRedisKV(redisClient)
				worker.RegisterMonitorHandler(mux, scraperService, monitorKV, logger.Log)
				monitorClient := asynq.NewClient(mustRedisOpt(cfg))
				defer monitorClient.Close()
				go worker.StartMonitorScheduler(ctx, monitorKV, monitorClient, scraperService, logger.Log)

				// Start (not Run): Run installs its own signal handler and
				// blocks, which would race with the HTTP server's shutdown
				// and leave the ordering below up to chance.
				if err := workerServer.Start(mux); err != nil {
					return fmt.Errorf("embedded worker failed to start: %w", err)
				}
			}
		}
	} else {
		logger.Log.Warn("Redis URL not configured, asynchronous crawling disabled")
	}

	// 4. Init Router
	router := api.NewRouter(cfg, logger.Log, scrapeHandler, crawlHandler, searchHandler, redisClient)

	// 5. Run Server
	//
	// Timeouts are deliberate:
	//   - ReadHeaderTimeout guards against slowloris-style clients that
	//     open a connection and trickle headers, which would otherwise pin
	//     a goroutine and an FD indefinitely under load.
	//   - ReadTimeout bounds how long a request body may take to arrive.
	//   - IdleTimeout reaps keep-alive connections that are not being used,
	//     so a burst of clients doesn't leave a pile of idle FDs behind.
	//   - WriteTimeout stays 0 (disabled): a scrape legitimately takes tens
	//     of seconds, and Go's WriteTimeout covers the whole handler span,
	//     so a value here would cut real work. The scraper enforces its own
	//     per-engine deadlines instead.
	srv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	logger.Log.Info("Server listening", "addr", srv.Addr)

	// 6. Block until the server dies or a signal arrives.
	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case <-ctx.Done():
		logger.Log.Info("Shutdown signal received, draining")
	}

	// Stop reacting to further signals so a second Ctrl-C can still force
	// an exit rather than being swallowed.
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout())
	defer cancel()

	// Start the worker drain now rather than after the HTTP drain, so the two
	// overlap.
	//
	// Asynq's processor polls with an uninterruptible sleep of up to
	// TaskCheckInterval (15s here, chosen to stay inside Upstash's free
	// command budget), and Shutdown blocks until that sleep returns even when
	// every queue is empty. Serialized after the HTTP drain, that stall *was*
	// the whole shutdown: the timeout fired every time with nothing in
	// flight. Overlapped, it costs no extra wall-clock in the common case.
	//
	// Tasks enqueued by requests that are still finishing are not lost —
	// they are in Redis, and the next process to start picks them up.
	// workerServer is nil when Redis is absent or DISABLE_WORKER is set, in
	// which case there is nothing to drain and nothing to report.
	var workerDone chan struct{}
	if workerServer != nil {
		workerDone = make(chan struct{})
		go func() {
			defer close(workerDone)
			workerServer.Shutdown()
		}()
	}

	// Drain HTTP: stop accepting new connections, let in-flight scrapes
	// finish and respond.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Log.Warn("HTTP shutdown did not complete cleanly", "error", err)
	} else {
		logger.Log.Info("HTTP server stopped")
	}

	// Then give the worker a short grace period on top. Shutdown has already
	// stopped it dequeuing new tasks, so anything it is still holding is
	// either finishing or will be re-queued; see workerDrainGrace.
	if workerDone != nil {
		graceCtx, graceCancel := context.WithTimeout(context.Background(), workerDrainGrace)
		defer graceCancel()
		select {
		case <-workerDone:
			logger.Log.Info("Embedded worker stopped")
		case <-graceCtx.Done():
			logger.Log.Info("Embedded worker still draining; exiting anyway, in-flight tasks will be retried",
				"grace", workerDrainGrace)
		}
	}

	// Deferred cleanup (browser, Redis, asynq clients) runs from here.
	logger.Log.Info("Shutdown complete")
	return nil
}
