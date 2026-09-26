package config

import (
	"fmt"
	"net"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	App    AppConfig    `mapstructure:"app"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Brave  BraveConfig  `mapstructure:"brave"`
	Search SearchConfig `mapstructure:"search"`
}

type BraveConfig struct {
	APIKey string `mapstructure:"api_key"`
}

// SearchConfig holds optional search backend configuration.
type SearchConfig struct {
	// SearXNGEndpoint, when set, points at a self-hosted SearXNG instance
	// (e.g. "http://localhost:8888") used as the primary search backend.
	// SearXNG aggregates many engines, so it is more stable than scraping a
	// single engine and costs nothing beyond one container.
	SearXNGEndpoint string `mapstructure:"searxng_endpoint"`
	// StealthEnabled, when true, enables the StealthService fallback that
	// reuses the shared ChromedpScraper as a BrowserFetcher. Bound to
	// STEALTH_ENABLED via Viper; see Load().
	StealthEnabled bool `mapstructure:"stealth_enabled"`
	// NativeEnabled (default true) builds the in-house engine layer from
	// the YAML registry (SEARCH_NATIVE_ENABLED).
	NativeEnabled bool `mapstructure:"native_enabled"`
	// CompatAddr, when set, starts the SearXNG JSON API listener
	// (SEARCH_COMPAT_ADDR, e.g. ":7435"). Empty = off.
	CompatAddr string `mapstructure:"compat_addr"`
	// EnginesPath overrides the bundled engine registry (SEARCH_ENGINES_PATH).
	EnginesPath string `mapstructure:"engines_path"`
	// WebshareAPIKey fetches the Webshare proxy list via their API
	// (WEBSHARE_API_KEY). Dummy values in .env.example.
	WebshareAPIKey string `mapstructure:"webshare_api_key"`
	// WebshareProxyURL is an explicit proxy endpoint list (comma-separated),
	// e.g. a backbone rotating URL — wins over API discovery
	// (WEBSHARE_PROXY_URL).
	WebshareProxyURL string `mapstructure:"webshare_proxy_url"`
	// ProxyTier1/ProxyTier2 are escalation groups (SEARCH_PROXY_TIER1/2).
	ProxyTier1 string `mapstructure:"proxy_tier1"`
	ProxyTier2 string `mapstructure:"proxy_tier2"`
	// ProxyBudgetGB hard-caps monthly proxy egress per process
	// (SEARCH_PROXY_BUDGET_GB, default 1, 0 = unlimited — plan R3).
	ProxyBudgetGB string `mapstructure:"proxy_budget_gb"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug, release, test
}

type AppConfig struct {
	LogLevel string `mapstructure:"loglevel"` // debug, info, warn, error

	// ChromeRecycleAfter restarts the Chrome allocator after this many
	// scrapes to bound browser memory growth (0 = default 100).
	ChromeRecycleAfter int `mapstructure:"chrome_recycle_after"`

	// ScreenshotMaxHeight caps full-page screenshot height in CSS pixels.
	// Taller pages are captured top-anchored and flagged truncated
	// (0 = default 16384, values above 32768 are clamped).
	ScreenshotMaxHeight int `mapstructure:"screenshot_max_height"`

	// APIKeys, when non-empty, enables X-API-Key auth on /v1/*.
	APIKeys []string `mapstructure:"api_keys"`

	// RateLimitRPM caps requests per client per minute (0 = unlimited).
	RateLimitRPM int `mapstructure:"rate_limit_rpm"`
}

type RedisConfig struct {
	URL      string `mapstructure:"url"`
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`

	// Upstash REST API credentials.
	// When set (and REDIS_URL is empty), the standard Redis URL is
	// derived automatically as:
	//   rediss://default:<RestToken>@<host-from-RestURL>:7434
	RestURL   string `mapstructure:"rest_url"`
	RestToken string `mapstructure:"rest_token"`
}

func Load() (*Config, error) {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		// .env file not found or error, continue with env vars
	}

	v := viper.New()

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults
	v.SetDefault("server.port", "7431")
	v.SetDefault("server.mode", "debug")
	v.SetDefault("app.loglevel", "info")
	v.SetDefault("app.chrome_recycle_after", 100)
	v.SetDefault("app.screenshot_max_height", 16384)
	v.SetDefault("app.api_keys", "")
	v.SetDefault("app.rate_limit_rpm", 0)
	v.SetDefault("redis.url", "")
	v.SetDefault("redis.host", "")
	v.SetDefault("redis.port", "")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.rest_url", "")
	v.SetDefault("redis.rest_token", "")
	v.SetDefault("brave.api_key", "")
	v.SetDefault("search.searxng_endpoint", "")
	v.SetDefault("search.stealth_enabled", false)
	v.SetDefault("search.native_enabled", true)
	v.SetDefault("search.compat_addr", "")
	v.SetDefault("search.engines_path", "")
	v.SetDefault("search.webshare_api_key", "")
	v.SetDefault("search.webshare_proxy_url", "")
	v.SetDefault("search.proxy_tier1", "")
	v.SetDefault("search.proxy_tier2", "")
	v.SetDefault("search.proxy_budget_gb", "1")

	// Custom bindings
	v.BindEnv("brave.api_key", "BRAVE_SEARCH_API_KEY")
	v.BindEnv("search.searxng_endpoint", "SEARXNG_ENDPOINT")
	v.BindEnv("search.stealth_enabled", "STEALTH_ENABLED")
	v.BindEnv("search.native_enabled", "SEARCH_NATIVE_ENABLED")
	v.BindEnv("search.compat_addr", "SEARCH_COMPAT_ADDR")
	v.BindEnv("search.engines_path", "SEARCH_ENGINES_PATH")
	v.BindEnv("search.webshare_api_key", "WEBSHARE_API_KEY")
	v.BindEnv("search.webshare_proxy_url", "WEBSHARE_PROXY_URL")
	v.BindEnv("search.proxy_tier1", "SEARCH_PROXY_TIER1")
	v.BindEnv("search.proxy_tier2", "SEARCH_PROXY_TIER2")
	v.BindEnv("search.proxy_budget_gb", "SEARCH_PROXY_BUDGET_GB")
	v.BindEnv("redis.rest_url", "UPSTASH_REDIS_REST_URL")
	v.BindEnv("redis.rest_token", "UPSTASH_REDIS_REST_TOKEN")

	// Aliases for app.* — canonical is APP_*, but accept legacy names too
	v.BindEnv("app.loglevel", "APP_LOGLEVEL", "LOG_LEVEL")
	v.BindEnv("app.api_keys", "APP_API_KEYS", "API_KEYS")
	v.BindEnv("app.rate_limit_rpm", "APP_RATE_LIMIT_RPM", "RATE_LIMIT_RPM")

	// Render injects PORT; Tomoshibi reads SERVER_PORT — accept either.
	v.BindEnv("server.port", "SERVER_PORT", "PORT")

	// No need for ReadInConfig since we use env vars

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse comma-separated API keys from the environment.
	if raw := v.GetString("app.api_keys"); raw != "" {
		cfg.App.APIKeys = nil
		for _, k := range strings.Split(raw, ",") {
			if k = strings.TrimSpace(k); k != "" {
				cfg.App.APIKeys = append(cfg.App.APIKeys, k)
			}
		}
	}

	// Construct Redis URL if not set but individual fields are present
	if cfg.Redis.URL == "" && cfg.Redis.Host != "" {
		port := cfg.Redis.Port
		if port == "" {
			port = "7434"
		}

		addr := net.JoinHostPort(cfg.Redis.Host, port)
		if cfg.Redis.Password != "" {
			cfg.Redis.URL = fmt.Sprintf("redis://:%s@%s", cfg.Redis.Password, addr)
		} else {
			cfg.Redis.URL = fmt.Sprintf("redis://%s", addr)
		}
	}

	// Derive Redis URL from Upstash REST credentials (standard env vars
	// set by Upstash console).  Only used when nothing else is set.
	if cfg.Redis.URL == "" && cfg.Redis.RestURL != "" && cfg.Redis.RestToken != "" {
		// UPSTASH_REDIS_REST_URL = https://<name>.upstash.io
		host := strings.TrimPrefix(cfg.Redis.RestURL, "https://")
		host = strings.TrimPrefix(host, "http://")
		// Strip trailing slash if present
		host = strings.TrimRight(host, "/")
		// Upstash Redis uses the same hostname and token, port 7434 with TLS
		cfg.Redis.URL = fmt.Sprintf("rediss://default:%s@%s:7434", cfg.Redis.RestToken, host)
	}

	return &cfg, nil
}
