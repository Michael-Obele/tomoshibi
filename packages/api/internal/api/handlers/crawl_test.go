package handlers

import (
	"testing"

	"github.com/hibiken/asynq"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestExtractTitle_FallbackChain(t *testing.T) {
	tests := []struct {
		name   string
		result domain.ScrapeResult
		want   string
	}{
		{
			name: "h1 wins",
			result: domain.ScrapeResult{
				URL:      "https://example.com/docs",
				HTML:     `<html><head><title>Tag Title</title></head><body><h1>Header One</h1></body></html>`,
				Markdown: "# Markdown Title",
			},
			want: "Header One",
		},
		{
			name: "title tag fallback",
			result: domain.ScrapeResult{
				URL:      "https://example.com/docs",
				HTML:     `<html><head><title>Tag Title</title></head><body><p>no headings</p></body></html>`,
				Markdown: "",
			},
			want: "Tag Title",
		},
		{
			name: "og:title fallback",
			result: domain.ScrapeResult{
				URL:  "https://example.com/share",
				HTML: `<html><head><meta property="og:title" content="Open Graph Title"></head><body></body></html>`,
			},
			want: "Open Graph Title",
		},
		{
			name: "any heading level fallback",
			result: domain.ScrapeResult{
				URL:  "https://example.com/deep",
				HTML: `<html><body><h2>Second Level</h2></body></html>`,
			},
			want: "Second Level",
		},
		{
			name: "markdown h1 fallback",
			result: domain.ScrapeResult{
				URL:      "https://example.com/md",
				HTML:     "",
				Markdown: "Intro text\n# Markdown Title\nBody",
			},
			want: "Markdown Title",
		},
		{
			// books.toscrape-style pages: no headings, no <title> — the
			// hostname fallback guarantees a non-empty title.
			name: "hostname fallback",
			result: domain.ScrapeResult{
				URL:      "https://books.toscrape.com/index.html",
				HTML:     `<html><body><p>**1000** results</p></body></html>`,
				Markdown: "**1000** results",
			},
			want: "books.toscrape.com",
		},
		{
			name:   "empty result",
			result: domain.ScrapeResult{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractTitle(tt.result); got != tt.want {
				t.Errorf("extractTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTaskStateLabel_ArchivedMapsToFailed(t *testing.T) {
	if got := taskStateLabel(asynq.TaskStateArchived); got != "failed" {
		t.Errorf("archived should map to 'failed', got %q", got)
	}
	for state, want := range map[asynq.TaskState]string{
		asynq.TaskStateActive:    "active",
		asynq.TaskStatePending:   "pending",
		asynq.TaskStateCompleted: "completed",
	} {
		if got := taskStateLabel(state); got != want {
			t.Errorf("taskStateLabel(%v) = %q, want %q", state, got, want)
		}
	}
}
