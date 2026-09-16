package handlers

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// maxBatchURLs caps how many URLs a single batch may contain.
const maxBatchURLs = 20

// BatchRequest is the request body for POST /v1/batch/scrape.
type BatchRequest struct {
	URLs []string `json:"urls" binding:"required,min=1,max=20"`
}

// BatchTask is one task within a batch.
type BatchTask struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// BatchResponse is returned by POST /v1/batch/scrape.
type BatchResponse struct {
	BatchID string      `json:"batch_id"`
	Tasks   []BatchTask `json:"tasks"`
}

// BatchStatusResponse is returned by GET /v1/batch/:id.
type BatchStatusResponse struct {
	BatchID   string      `json:"batch_id"`
	Total     int         `json:"total"`
	Completed int         `json:"completed"`
	Failed    int         `json:"failed"`
	Tasks     []BatchTask `json:"tasks"`
}

// BatchHandler manages multi-URL batch scraping via Asynq + Redis.
type BatchHandler struct {
	client    *asynq.Client
	inspector *asynq.Inspector
	redis     *redis.Client
}

// NewBatchHandler constructs a BatchHandler from a Redis URL.
func NewBatchHandler(redisAddr string) (*BatchHandler, error) {
	u, err := url.Parse(redisAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}
	password, _ := u.User.Password()
	redisOpt := asynq.RedisClientOpt{Addr: u.Host, Password: password}
	if u.Scheme == "rediss" {
		redisOpt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	rdbOpt, err := redis.ParseURL(redisAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	return &BatchHandler{
		client:    asynq.NewClient(redisOpt),
		inspector: asynq.NewInspector(redisOpt),
		redis:     redis.NewClient(rdbOpt),
	}, nil
}

// Close releases the handler's connections.
func (h *BatchHandler) Close() {
	h.client.Close()
	h.inspector.Close()
	_ = h.redis.Close()
}

// EnqueueBatch godoc
// @Summary      Scrape multiple URLs asynchronously
// @Description  Enqueues one scrape task per URL and returns a batch ID for status polling.
// @Tags         batch
// @Accept       json
// @Produce      json
// @Param        body   body      BatchRequest  true  "JSON request body"
// @Success      202    {object}  BatchResponse
// @Failure      400    {object}  map[string]interface{}
// @Failure      500    {object}  map[string]interface{}
// @Router       /batch/scrape [post]
func (h *BatchHandler) EnqueueBatch(c *gin.Context) {
	var req BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.URLs) > maxBatchURLs {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("too many URLs (max %d)", maxBatchURLs)})
		return
	}

	batchID, err := newBatchID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate batch id"})
		return
	}

	tasks := make([]BatchTask, 0, len(req.URLs))
	for _, u := range req.URLs {
		task, err := worker.NewScrapeTask(u, false, false, false, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
			return
		}
		info, err := h.client.Enqueue(task)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue task"})
			return
		}
		tasks = append(tasks, BatchTask{ID: info.ID, URL: u})
	}

	// Persist the batch record for status aggregation.
	data, _ := json.Marshal(tasks)
	if err := h.redis.Set(c.Request.Context(), "batch:"+batchID, data, 7*24*time.Hour).Err(); err != nil {
		// Non-fatal: status still derivable from task IDs returned inline.
		_ = err
	}

	c.JSON(http.StatusAccepted, BatchResponse{BatchID: batchID, Tasks: tasks})
}

// GetBatchStatus godoc
// @Summary      Get the status of a batch scrape
// @Description  Aggregates the Asynq state of every task in the batch.
// @Tags         batch
// @Produce      json
// @Param        id     path      string  true  "The batch ID"
// @Success      200    {object}  BatchStatusResponse
// @Failure      404    {object}  map[string]interface{}
// @Router       /batch/{id} [get]
func (h *BatchHandler) GetBatchStatus(c *gin.Context) {
	id := c.Param("id")

	data, err := h.redis.Get(c.Request.Context(), "batch:"+id).Bytes()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	tasks := []BatchTask{}
	if err := json.Unmarshal(data, &tasks); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "corrupt batch record"})
		return
	}

	var completed, failed int
	for i := range tasks {
		info, err := h.inspector.GetTaskInfo("default", tasks[i].ID)
		if err != nil {
			// A task that no longer exists (or a queue that has no keys
			// yet) is done — count it as completed rather than silently
			// dropping it. A transient backend error, though, means the
			// whole status is unreliable right now — surface that as 503
			// instead of under-reporting.
			if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
				completed++
				continue
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":  "batch status temporarily unavailable",
				"detail": err.Error(),
			})
			return
		}
		switch info.State {
		case asynq.TaskStateCompleted:
			completed++
		case asynq.TaskStateArchived, asynq.TaskStateRetry:
			failed++
		}
	}

	c.JSON(http.StatusOK, BatchStatusResponse{
		BatchID:   id,
		Total:     len(tasks),
		Completed: completed,
		Failed:    failed,
		Tasks:     tasks,
	})
}

// newBatchID returns a random 16-byte hex identifier.
func newBatchID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
