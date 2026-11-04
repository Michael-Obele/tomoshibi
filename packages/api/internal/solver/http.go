// Package solver talks to an out-of-process anti-bot addon over HTTP.
//
// The addon is deliberately not linked into the core binary: a captcha or
// challenge solver carries a browser stack, provider keys, and often a
// commercial licence. Keeping it behind a URL means the MIT core stays small
// and the addon can be distributed, versioned, and billed separately.
package solver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// maxSolutionBytes caps the addon response. A solved page is HTML; anything
// larger is a misbehaving addon, not content.
const maxSolutionBytes = 8 << 20

// HTTP is a ChallengeSolver backed by a JSON endpoint.
type HTTP struct {
	url    string
	token  string
	client *http.Client
}

// NewHTTP builds a solver client. The endpoint is operator configuration, not
// caller input, so it is intentionally not routed through the SSRF guard: a
// sidecar solver usually lives on localhost.
func NewHTTP(url, token string) *HTTP {
	return &HTTP{
		url:   url,
		token: token,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Solve posts the challenge to the addon and returns the recovered HTML.
func (h *HTTP) Solve(ctx context.Context, req domain.ChallengeRequest) (*domain.ChallengeSolution, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal challenge: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build solver request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if h.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.token)
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call solver: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("solver returned HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(detail))
	}

	var solution domain.ChallengeSolution
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSolutionBytes)).Decode(&solution); err != nil {
		return nil, fmt.Errorf("decode solver response: %w", err)
	}
	if solution.HTML == "" {
		return nil, fmt.Errorf("solver returned an empty solution")
	}
	return &solution, nil
}
