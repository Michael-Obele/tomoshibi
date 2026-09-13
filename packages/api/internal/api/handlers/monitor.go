package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/Michael-Obele/tomoshibi/internal/worker"
)

// minMonitorInterval is the minimum allowed monitor interval in seconds.
const minMonitorInterval = 3600

// monitorPrefix matches the worker package's Redis key prefix.
const monitorPrefix = "monitor:"

// MonitorRequest is the request body for POST /v1/monitor.
type MonitorRequest struct {
	URL             string `json:"url" binding:"required,url"`
	IntervalSeconds int    `json:"interval_seconds"`
	WebhookURL      string `json:"webhook_url,omitempty"`
	WebhookSecret   string `json:"webhook_secret,omitempty"`
}

// MonitorResponse is returned by POST /v1/monitor.
type MonitorResponse struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
	NextCheck       string `json:"next_check"`
}

// MonitorHandler manages change-tracking monitors.
type MonitorHandler struct {
	redis *redis.Client
	kv    worker.KV
}

// NewMonitorHandler constructs a MonitorHandler from a Redis URL.
func NewMonitorHandler(redisAddr string) (*MonitorHandler, error) {
	opt, err := redis.ParseURL(redisAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}
	client := redis.NewClient(opt)
	return &MonitorHandler{redis: client, kv: worker.NewRedisKV(client)}, nil
}

// Close releases the handler's Redis connection.
func (h *MonitorHandler) Close() {
	_ = h.redis.Close()
}

// CreateMonitor godoc
// @Summary      Create a change-tracking monitor
// @Description  Scrapes the URL on an interval, hashes the markdown, and fires a webhook when content changes. First check records the baseline.
// @Tags         monitor
// @Accept       json
// @Produce      json
// @Param        body   body      MonitorRequest  true  "JSON request body"
// @Success      201    {object}  MonitorResponse
// @Failure      400    {object}  map[string]interface{}
// @Failure      500    {object}  map[string]interface{}
// @Router       /monitor [post]
func (h *MonitorHandler) CreateMonitor(c *gin.Context) {
	var req MonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.IntervalSeconds < minMonitorInterval {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("interval_seconds must be at least %d", minMonitorInterval)})
		return
	}
	if req.WebhookURL != "" {
		if _, err := url.ParseRequestURI(req.WebhookURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook_url"})
			return
		}
	}

	id, err := newMonitorID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate monitor id"})
		return
	}

	cfg := worker.MonitorConfig{
		ID:              id,
		URL:             req.URL,
		IntervalSeconds: req.IntervalSeconds,
		WebhookURL:      req.WebhookURL,
		WebhookSecret:   req.WebhookSecret,
		NextCheck:       time.Now(), // due immediately → baseline on next tick
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist monitor"})
		return
	}
	if err := h.redis.Set(c.Request.Context(), monitorPrefix+id, data, 0).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist monitor"})
		return
	}

	c.JSON(http.StatusCreated, MonitorResponse{
		ID:              id,
		URL:             cfg.URL,
		IntervalSeconds: cfg.IntervalSeconds,
		NextCheck:       cfg.NextCheck.UTC().Format(time.RFC3339),
	})
}

// GetMonitor godoc
// @Summary      Get monitor status
// @Description  Returns the monitor configuration, last content hash, and next check time.
// @Tags         monitor
// @Produce      json
// @Param        id     path      string  true  "The monitor ID"
// @Success      200    {object}  map[string]interface{}
// @Failure      404    {object}  map[string]interface{}
// @Router       /monitor/{id} [get]
func (h *MonitorHandler) GetMonitor(c *gin.Context) {
	id := c.Param("id")
	raw, err := h.redis.Get(c.Request.Context(), monitorPrefix+id).Result()
	if err != nil {
		// Only a genuine miss is a 404. A Redis outage or pool exhaustion
		// must not masquerade as "monitor not found" — that is a false
		// negative that makes clients delete/recreate monitors that exist.
		if err == redis.Nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  "monitor status temporarily unavailable",
			"detail": err.Error(),
		})
		return
	}
	var cfg worker.MonitorConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "corrupt monitor record"})
		return
	}

	hash, _ := h.redis.Get(c.Request.Context(), monitorPrefix+id+":hash").Result()
	c.JSON(http.StatusOK, gin.H{
		"id":               cfg.ID,
		"url":              cfg.URL,
		"interval_seconds": cfg.IntervalSeconds,
		"webhook_url":      cfg.WebhookURL,
		"last_hash":        hash,
		"next_check":       cfg.NextCheck.UTC().Format(time.RFC3339),
	})
}

// DeleteMonitor godoc
// @Summary      Delete a monitor
// @Description  Stops monitoring and removes the monitor record.
// @Tags         monitor
// @Param        id     path      string  true  "The monitor ID"
// @Success      204
// @Failure      404    {object}  map[string]interface{}
// @Router       /monitor/{id} [delete]
func (h *MonitorHandler) DeleteMonitor(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	removed, err := h.redis.Del(ctx, monitorPrefix+id, monitorPrefix+id+":hash").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete monitor"})
		return
	}
	if removed == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// newMonitorID returns a random 16-byte hex identifier.
func newMonitorID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
