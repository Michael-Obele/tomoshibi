package main

import (
	"fmt"
	"os"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/Michael-Obele/tomoshibi/internal/config"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/Michael-Obele/tomoshibi/internal/worker"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
)

func main() {
	code, err := run()
	if err != nil {
		if logger.Log != nil {
			logger.Log.Error("Worker exited with error", "error", err)
		} else {
			fmt.Printf("Worker exited with error: %v\n", err)
		}
	}
	os.Exit(code)
}

// run holds the worker lifecycle. It returns an exit code rather than
// calling os.Exit directly so deferred cleanup (browser, Redis) runs —
// otherwise Chrome subprocesses outlive the process on every restart.
func run() (int, error) {
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		return 1, fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Initialize Logger
	logger.Init(cfg.App.LogLevel)
	logger.Log.Info("Starting Tomoshibi Worker")

	// Check if Redis is configured
	if cfg.Redis.URL == "" {
		logger.Log.Warn("Redis URL not configured, worker cannot start. Use synchronous scraping only.")
		return 0, nil
	}

	// 3. Initialize Scrapers
	// Create standard go-redis client for caching (asynq uses its own connection)
	redisOpt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return 1, fmt.Errorf("failed to parse Redis URI: %w", err)
	}
	redisClient := redis.NewClient(redisOpt)
	defer redisClient.Close()

	collyScraper := scraper.NewCollyScraper()
	chromedpScraper := scraper.NewChromedpScraper()
	defer chromedpScraper.Close()
	scraperService := scraper.NewService(collyScraper, chromedpScraper, redisClient)

	// 4. Initialize Asynq Server
	srv := worker.NewServer(cfg, logger.Log)

	// 5. Register Handlers
	mux := asynq.NewServeMux()
	worker.RegisterHandlers(mux, scraperService, logger.Log)

	// 6. Run. asynq's Run installs its own SIGTERM/SIGINT handler and drains
	// in-flight tasks before returning, so the standalone worker needs no
	// signal wiring of its own — unlike cmd/api, which must coordinate the
	// drain with an HTTP server.
	logger.Log.Info("Worker is running...")
	if err := srv.Run(mux); err != nil {
		return 1, fmt.Errorf("could not run worker server: %w", err)
	}

	logger.Log.Info("Worker shutdown complete")
	return 0, nil
}
