package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/scraper"
	"github.com/hibiken/asynq"
)

type ScrapeTaskHandler struct {
	scraper *scraper.Service
	logger  *slog.Logger
}

func NewScrapeTaskHandler(scraper *scraper.Service, logger *slog.Logger) *ScrapeTaskHandler {
	return &ScrapeTaskHandler{
		scraper: scraper,
		logger:  logger,
	}
}

func (h *ScrapeTaskHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload ScrapePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w, task_id=%s", err, t.ResultWriter().TaskID())
	}

	h.logger.Info("Processing scrape task", "url", payload.URL, "render", payload.Render, "mode", payload.Mode, "screenshot", payload.Screenshot, "images", payload.Images, "task_id", t.ResultWriter().TaskID())

	// Backward compatibility mapping
	mode := payload.Mode
	if payload.Render {
		mode = "dynamic"
	}
	if mode == "" {
		mode = "smart"
	}

	// Parse image format
	imageFormat := domain.ImageFormatURL
	switch payload.ImageFormat {
	case "blob":
		imageFormat = domain.ImageFormatBlob
	case "url":
		imageFormat = domain.ImageFormatURL
	}

	result, err := h.scraper.Scrape(ctx, payload.URL, mode, domain.ScrapeOptions{
		Screenshot:  payload.Screenshot,
		Images:      payload.Images,
		ImageFormat: imageFormat,
	})
	if err != nil {
		h.logger.Error("Scraping failed", "error", err, "url", payload.URL, "task_id", t.ResultWriter().TaskID())
		return fmt.Errorf("scraping failed: %w", err)
	}

	h.logger.Info("Scrape successful", "url", payload.URL, "task_id", t.ResultWriter().TaskID(), "engine", result.Metadata["engine"])

	// In a real scenario, you might store the result in a database or object storage here.
	// For now, we'll just log slightly more detail or potentially return it (Asynq result writing is limited to a byte slice).

	// We can write the result ID or summary to the task info
	_, _ = t.ResultWriter().Write(fmt.Appendf(nil, "Scraped %s successfully", payload.URL))

	// TODO: Save 'result' to persistent storage so it can be retrieved via /v1/crawl/:id
	// For Phase 3, we might largely rely on logs or a simple in-memory/redis cache if we want to show results.
	// To keep it simple in Phase 3, we will just succeed. The 'GET /v1/crawl/:id' will mostly check status.

	return nil
}
