package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/config"
	"github.com/Michael-Obele/tomoshibi/internal/telemetry"
	"github.com/gin-gonic/gin"
)

// InsightsHandler serves GET /v1/insights — the local telemetry aggregate.
type InsightsHandler struct {
	cfg config.TelemetryConfig
}

// NewInsightsHandler builds the handler from telemetry configuration.
func NewInsightsHandler(cfg config.TelemetryConfig) *InsightsHandler {
	return &InsightsHandler{cfg: cfg}
}

// Insights godoc
// @Summary      Local search telemetry
// @Description  Aggregates the local JSONL telemetry: per-engine scorecard (ok/empty/blocked/timeout), chain stats (fallbacks, weak gates), latency percentiles and recent errors. Local-only — nothing leaves the machine. Returns {"enabled":false} when TELEMETRY_ENABLED=false.
// @Tags         search
// @Produce      json
// @Param        hours query int false "Aggregation window in hours (default 24, max 720)"
// @Success      200 {object} map[string]interface{}
// @Router       /v1/insights [get]
func (h *InsightsHandler) Insights(c *gin.Context) {
	if !h.cfg.Enabled {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "telemetry disabled — set TELEMETRY_ENABLED=true",
		})
		return
	}
	hours, err := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if err != nil || hours < 1 {
		hours = 24
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	ins, err := telemetry.Read(h.cfg.Dir, time.Duration(hours)*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "read telemetry: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "insights": ins})
}
