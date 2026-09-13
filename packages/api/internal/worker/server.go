package worker

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/hibiken/asynq"
	"github.com/Michael-Obele/tomoshibi/internal/config"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
)

type asynqLogger struct {
	logger *slog.Logger
}

// AsynqLogger is the exported alias for backward compatibility.
//
// Deprecated: use asynqLogger internally; kept exported so external callers
// can still reference the type if needed.
type AsynqLogger = asynqLogger

func (l *asynqLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

func (l *asynqLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

func (l *asynqLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

func (l *asynqLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

func (l *asynqLogger) Fatal(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
	os.Exit(1)
}

// RedisClientOpt builds an asynq.RedisClientOpt from a redis:// or
// rediss:// URL, including TLS config for the latter.
func RedisClientOpt(redisURL string) (asynq.RedisClientOpt, error) {
	u, err := url.Parse(redisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, fmt.Errorf("failed to parse redis url: %w", err)
	}
	password, _ := u.User.Password()
	opt := asynq.RedisClientOpt{Addr: u.Host, Password: password}
	if u.Scheme == "rediss" {
		opt.TLSConfig = &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		}
	}
	return opt, nil
}

func NewServer(cfg *config.Config, logger *slog.Logger) *asynq.Server {
	redisOpt, err := RedisClientOpt(cfg.Redis.URL)
	if err != nil {
		panic(fmt.Sprintf("failed to parse redis url: %v", err))
	}

	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 5, // Reduced for hobby tier (256MB VM)
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
			// 15s poll interval keeps Redis commands well within Upstash
			// free tier (500K/mo): ~172K checks/mo vs 2.6M at 1s.
			//
			// Trade-off worth knowing: asynq's processor sleeps up to this
			// long uninterruptibly when queues are empty, and Shutdown blocks
			// until that sleep returns. So this value is also a floor on
			// shutdown latency — cmd/api overlaps the worker drain with the
			// HTTP drain to hide it.
			TaskCheckInterval: 15 * time.Second,
			// How long active workers get to finish before asynq aborts them
			// and pushes their tasks back to Redis. Must stay below the
			// process-level SHUTDOWN_TIMEOUT (default 20s) so the re-queue
			// actually happens instead of being cut off by a SIGKILL.
			ShutdownTimeout: 10 * time.Second,
			Logger:          &asynqLogger{logger: logger},
		},
	)

	return srv
}

func RegisterHandlers(mux *asynq.ServeMux, scraper *scraper.Service, logger *slog.Logger) {
	scrapeHandler := NewScrapeTaskHandler(scraper, logger)
	mux.HandleFunc(TypeScrape, scrapeHandler.ProcessTask)

	crawlHandler := NewCrawlTaskHandler(scraper, logger)
	mux.HandleFunc(TypeCrawl, crawlHandler.ProcessTask)
}

// RegisterMonitorHandler registers the monitor:check task handler and
// returns the handler (its KV is needed by the scheduler).
func RegisterMonitorHandler(
	mux *asynq.ServeMux,
	scraper *scraper.Service,
	kv KV,
	logger *slog.Logger,
) *MonitorTaskHandler {
	handler := NewMonitorTaskHandler(scraper, kv, logger)
	mux.HandleFunc(TypeMonitorCheck, handler.ProcessTask)
	return handler
}
