package domain

import "context"

// ChallengeRequest describes a blocked fetch handed to an optional solver
// addon. HTML is the challenge page as fetched, so the addon can identify the
// vendor and decide how to proceed.
type ChallengeRequest struct {
	URL        string `json:"url"`
	Vendor     string `json:"vendor,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	HTML       string `json:"html,omitempty"`
}

// ChallengeSolution is the post-challenge HTML the addon recovered. The core
// converts it through the same pipeline as a normal static scrape.
type ChallengeSolution struct {
	HTML string `json:"html"`
}

// ChallengeSolver is the seam for optional anti-bot addons.
//
// The core never imports an implementation. An operator enables one by
// pointing ANTIBOT_SOLVER_URL at an out-of-process service, which keeps the
// MIT core small and lets a private or paid addon ship its own browser stack,
// dependencies, and licence on its own schedule.
type ChallengeSolver interface {
	Solve(ctx context.Context, req ChallengeRequest) (*ChallengeSolution, error)
}
