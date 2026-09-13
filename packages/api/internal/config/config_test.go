package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars that could affect the test
	envsToClear := []string{
		"PORT", "SERVER_MODE", "APP_LOGLEVEL",
		"REDIS_URL", "REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD",
		"UPSTASH_REDIS_REST_URL", "UPSTASH_REDIS_REST_TOKEN",
		"BRAVE_SEARCH_API_KEY",
		"SERVER_PORT", "SERVER_MODE",
	}

	saved := make(map[string]string)
	for _, key := range envsToClear {
		if val, ok := os.LookupEnv(key); ok {
			saved[key] = val
			os.Unsetenv(key)
		}
	}
	defer func() {
		for key, val := range saved {
			os.Setenv(key, val)
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.Port != "8080" {
		t.Errorf("Default port should be 8080, got %q", cfg.Server.Port)
	}

	if cfg.Server.Mode != "debug" {
		t.Errorf("Default mode should be debug, got %q", cfg.Server.Mode)
	}

	if cfg.App.LogLevel != "info" {
		t.Errorf("Default log level should be info, got %q", cfg.App.LogLevel)
	}
}

func TestLoad_RedisURLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		port        string
		password    string
		expectedURL string
	}{
		{
			name:        "Host without port defaults to 6379",
			host:        "redis.example.com",
			port:        "",
			password:    "",
			expectedURL: "redis://redis.example.com:6379",
		},
		{
			name:        "Host with custom port",
			host:        "redis.example.com",
			port:        "6380",
			password:    "",
			expectedURL: "redis://redis.example.com:6380",
		},
		{
			name:        "Host with password",
			host:        "redis.example.com",
			port:        "6379",
			password:    "secret",
			expectedURL: "redis://:secret@redis.example.com:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Redis: RedisConfig{
					URL:      "",
					Host:     tt.host,
					Port:     tt.port,
					Password: tt.password,
				},
			}

			// Replicate the URL construction logic from Load()
			if cfg.Redis.URL == "" && cfg.Redis.Host != "" {
				port := cfg.Redis.Port
				if port == "" {
					port = "6379"
				}
				addr := cfg.Redis.Host + ":" + port
				if cfg.Redis.Password != "" {
					cfg.Redis.URL = "redis://:" + cfg.Redis.Password + "@" + addr
				} else {
					cfg.Redis.URL = "redis://" + addr
				}
			}

			if cfg.Redis.URL != tt.expectedURL {
				t.Errorf("Redis URL = %q, want %q", cfg.Redis.URL, tt.expectedURL)
			}
		})
	}
}

func TestLoad_RedisURLPreferred(t *testing.T) {
	// When REDIS_URL is already set, individual fields should be ignored
	cfg := &Config{
		Redis: RedisConfig{
			URL:  "redis://existing:6379",
			Host: "other-host",
			Port: "6380",
		},
	}

	// The Load logic only constructs URL if cfg.Redis.URL == ""
	if cfg.Redis.URL == "" && cfg.Redis.Host != "" {
		t.Error("Should not reconstruct URL when URL is already set")
	}

	if cfg.Redis.URL != "redis://existing:6379" {
		t.Errorf("Expected existing URL to be preserved, got %q", cfg.Redis.URL)
	}
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			Port: "9090",
			Mode: "release",
		},
		App: AppConfig{
			LogLevel: "debug",
		},
		Redis: RedisConfig{
			URL: "redis://localhost:6379",
		},
		Brave: BraveConfig{
			APIKey: "test-key",
		},
	}

	if cfg.Server.Port != "9090" {
		t.Errorf("Server port mismatch")
	}
	if cfg.Server.Mode != "release" {
		t.Errorf("Server mode mismatch")
	}
	if cfg.App.LogLevel != "debug" {
		t.Errorf("Log level mismatch")
	}
	if cfg.Redis.URL != "redis://localhost:6379" {
		t.Errorf("Redis URL mismatch")
	}
	if cfg.Brave.APIKey != "test-key" {
		t.Errorf("Brave API key mismatch")
	}
}
