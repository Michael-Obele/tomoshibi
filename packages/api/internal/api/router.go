package api

import (
	"log/slog"
	"net/http"

	"github.com/Michael-Obele/tomoshibi/internal/agentdocs"
	"github.com/Michael-Obele/tomoshibi/internal/api/handlers"
	"github.com/Michael-Obele/tomoshibi/internal/api/middleware"
	"github.com/Michael-Obele/tomoshibi/internal/config"
	"github.com/Michael-Obele/tomoshibi/internal/version"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func NewRouter(
	cfg *config.Config,
	logger *slog.Logger,
	scrapeHandler *handlers.ScrapeHandler,
	crawlHandler *handlers.CrawlHandler,
	searchHandler *handlers.SearchHandler,
	redisClient *redis.Client,
) *gin.Engine {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Filled in below once the embedded docs are loaded; "unknown" tells a caller
	// the build has no docs rather than an empty string.
	docsVersion := "unknown"

	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))

	// NOTE: the /bridge WebSocket is deliberately NOT mounted here. A WS
	// upgrade must hijack the connection before any wrapper writes a status,
	// and gin's response writer refuses that. main.go serves it from the root
	// mux, in front of this router.

	// Root health check — Fly.io uses this to verify the app is alive.
	// All API routes live under /v1.
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "tomoshibi",
			"version": version.Version,
			"status":  "ok",
			"docs":    "/swagger/index.html",
			"endpoints": gin.H{
				"scrape":   "/v1/scrape",
				"search":   "/v1/search",
				"env":      "/v1/env",
				"crawl":    "/v1/crawl",
				"map":      "/v1/map",
				"batch":    "/v1/batch/scrape",
				"monitor":  "/v1/monitor",
				"insights": "/v1/insights",
				"research": "/v1/research",
			},
			// The documentation an agent should read first. Unauthenticated like
			// this endpoint, and generated from the repo rather than written here:
			// see internal/agentdocs.
			"llms_txt":      "/llms.txt",
			"docs_manifest": "/docs",
			"docs_version":  docsVersion,
		})
	})

	// Agent documentation: /llms.txt (llmstxt.org v2), /llms-full.txt, /docs and
	// /docs/<slug>.md. Every route carries X-Docs-Version and an ETag, and the
	// content is generated from the tracked markdown by cmd/docsgen — so it
	// cannot describe a different API than the one answering here.
	if bundle, err := agentdocs.Load(); err != nil {
		// A broken docs embed is a build problem, not a request problem. Say so
		// loudly and keep the API serving: refusing to boot over documentation
		// would be worse than serving without it.
		logger.Error("Agent docs unavailable", "error", err)
	} else {
		bundle.Routes(r)
		docsVersion = bundle.Manifest().Version
	}

	// Liveness probes: unauthenticated so load balancers and uptime
	// monitors can check the API without an API key.
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "tomoshibi"})
	})
	r.GET("/v1/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "tomoshibi"})
	})

	// Swagger Docs mapping
	if cfg.Server.Mode == "debug" {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	} else {
		// Optionally, you might want to disable swagger entirely in release
		r.GET("/swagger/*any", func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Swagger available only in debug mode"})
		})
	}

	v1 := r.Group("/v1")
	v1.Use(middleware.APIKeyAuth(cfg.App.APIKeys))
	v1.Use(middleware.RateLimit(cfg.App.RateLimitRPM, redisClient))
	// After auth: per-request env overrides (X-Tomoshi-Env) only matter for
	// authenticated callers, and the parse is skipped entirely without the header.
	v1.Use(middleware.RequestEnv(logger))
	{
		v1.POST("/scrape", scrapeHandler.Scrape)
		v1.GET("/scrape", scrapeHandler.Scrape)
		v1.POST("/search", searchHandler.Search)
		v1.GET("/search", searchHandler.Search)
		// Research is search plus the pages behind it, in one call and one
		// budget: a composition of two things that already exist, not a new
		// pipeline.
		v1.POST("/research", searchHandler.Research)
		v1.GET("/research", searchHandler.Research)
		v1.GET("/insights", handlers.NewInsightsHandler(cfg.Telemetry).Insights)

		// Discovery for env forwarding: which env var names this backend
		// accepts per request in X-Tomoshi-Env (used by the MCP client).
		v1.GET("/env", handlers.Env)

		// URL discovery via sitemap/link traversal.
		v1.POST("/map", handlers.NewMapHandler().Map)

		// Only register crawl routes if Redis/crawl handler is available
		if crawlHandler != nil {
			v1.POST("/crawl", crawlHandler.EnqueueCrawl)
			v1.GET("/crawl/:id", crawlHandler.GetCrawlStatus)

			// Batch scrape also requires Redis (Asynq + batch records).
			if batchHandler, err := handlers.NewBatchHandler(cfg.Redis.URL); err == nil {
				v1.POST("/batch/scrape", batchHandler.EnqueueBatch)
				v1.GET("/batch/:id", batchHandler.GetBatchStatus)
			} else {
				logger.Warn("Batch scraping disabled", "error", err)
			}

			// Change-tracking monitors also require Redis.
			if monitorHandler, err := handlers.NewMonitorHandler(cfg.Redis.URL); err == nil {
				v1.POST("/monitor", monitorHandler.CreateMonitor)
				v1.GET("/monitor/:id", monitorHandler.GetMonitor)
				v1.DELETE("/monitor/:id", monitorHandler.DeleteMonitor)
			} else {
				logger.Warn("Monitor endpoints disabled", "error", err)
			}
		} else {
			// Return 503 Service Unavailable for crawl endpoints when Redis is not available
			v1.POST("/crawl", func(c *gin.Context) {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "Asynchronous crawling is not available - Redis connection required",
				})
			})
			v1.GET("/crawl/:id", func(c *gin.Context) {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "Asynchronous crawling is not available - Redis connection required",
				})
			})
		}
	}

	return r
}
