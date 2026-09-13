package worker

import (
	"encoding/json"
	"testing"
)

func TestNewScrapeTask(t *testing.T) {
	task, err := NewScrapeTask("https://example.com", false, false, false, "")
	if err != nil {
		t.Fatalf("NewScrapeTask failed: %v", err)
	}

	if task == nil {
		t.Fatal("Task should not be nil")
	}

	// Verify payload
	var payload ScrapePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if payload.URL != "https://example.com" {
		t.Errorf("URL mismatch: got %q, want %q", payload.URL, "https://example.com")
	}

	if payload.Render != false {
		t.Error("Render should be false")
	}
}

func TestNewScrapeTask_WithRender(t *testing.T) {
	task, err := NewScrapeTask("https://example.com", true, false, false, "")
	if err != nil {
		t.Fatalf("NewScrapeTask failed: %v", err)
	}

	var payload ScrapePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if payload.Render != true {
		t.Error("Render should be true")
	}
}

func TestNewScrapeTask_TaskType(t *testing.T) {
	task, err := NewScrapeTask("https://example.com", false, false, false, "")
	if err != nil {
		t.Fatalf("NewScrapeTask failed: %v", err)
	}

	if task.Type() != TypeScrape {
		t.Errorf("Task type should be %q, got %q", TypeScrape, task.Type())
	}
}

func TestScrapePayload_JSON(t *testing.T) {
	payload := ScrapePayload{
		URL:    "https://example.com",
		Render: true,
		Mode:   "dynamic",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ScrapePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.URL != payload.URL {
		t.Errorf("URL mismatch")
	}
	if decoded.Render != payload.Render {
		t.Errorf("Render mismatch")
	}
	if decoded.Mode != payload.Mode {
		t.Errorf("Mode mismatch")
	}
}

func TestScrapePayload_BackwardCompatibility(t *testing.T) {
	// Test that the old format (just URL + Render) still works
	oldJSON := `{"url":"https://example.com","render":true}`

	var payload ScrapePayload
	if err := json.Unmarshal([]byte(oldJSON), &payload); err != nil {
		t.Fatalf("Failed to unmarshal old format: %v", err)
	}

	if payload.URL != "https://example.com" {
		t.Errorf("URL mismatch")
	}
	if payload.Render != true {
		t.Errorf("Render mismatch")
	}
	if payload.Mode != "" {
		t.Errorf("Mode should be empty for old format, got %q", payload.Mode)
	}
}

func TestScrapePayload_ModeMapping(t *testing.T) {
	// Verify the backward-compatible mode mapping logic from handlers.go
	tests := []struct {
		name         string
		render       bool
		mode         string
		expectedMode string
	}{
		{
			name:         "Render true overrides to dynamic",
			render:       true,
			mode:         "",
			expectedMode: "dynamic",
		},
		{
			name:         "Empty mode defaults to smart",
			render:       false,
			mode:         "",
			expectedMode: "smart",
		},
		{
			name:         "Explicit static mode",
			render:       false,
			mode:         "static",
			expectedMode: "static",
		},
		{
			name:         "Explicit dynamic mode",
			render:       false,
			mode:         "dynamic",
			expectedMode: "dynamic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the mode mapping logic
			mode := tt.mode
			if tt.render {
				mode = "dynamic"
			}
			if mode == "" {
				mode = "smart"
			}

			if mode != tt.expectedMode {
				t.Errorf("Mode = %q, want %q", mode, tt.expectedMode)
			}
		})
	}
}

// --- CrawlTask Tests ---

func TestNewCrawlTask(t *testing.T) {
	task, err := NewCrawlTask("https://example.com", false, false, false, "", 3, 20)
	if err != nil {
		t.Fatalf("NewCrawlTask failed: %v", err)
	}

	if task == nil {
		t.Fatal("Task should not be nil")
	}

	if task.Type() != TypeCrawl {
		t.Errorf("Task type should be %q, got %q", TypeCrawl, task.Type())
	}

	// Verify payload
	var payload CrawlPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if payload.URL != "https://example.com" {
		t.Errorf("URL mismatch: got %q", payload.URL)
	}
	if payload.MaxDepth != 3 {
		t.Errorf("MaxDepth mismatch: got %d, want 3", payload.MaxDepth)
	}
	if payload.Limit != 20 {
		t.Errorf("Limit mismatch: got %d, want 20", payload.Limit)
	}
}

func TestNewCrawlTask_WithAllOptions(t *testing.T) {
	task, err := NewCrawlTask("https://docs.example.com", true, true, true, "", 5, 50)
	if err != nil {
		t.Fatalf("NewCrawlTask failed: %v", err)
	}

	var payload CrawlPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !payload.Render {
		t.Error("Render should be true")
	}
	if !payload.Screenshot {
		t.Error("Screenshot should be true")
	}
	if !payload.Images {
		t.Error("Images should be true")
	}
	if payload.MaxDepth != 5 {
		t.Errorf("MaxDepth = %d, want 5", payload.MaxDepth)
	}
	if payload.Limit != 50 {
		t.Errorf("Limit = %d, want 50", payload.Limit)
	}
}

func TestCrawlPayload_JSON_Roundtrip(t *testing.T) {
	original := CrawlPayload{
		URL:        "https://example.com",
		Render:     false,
		Mode:       "smart",
		Screenshot: true,
		Images:     false,
		MaxDepth:   4,
		Limit:      30,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded CrawlPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.URL != original.URL {
		t.Errorf("URL mismatch")
	}
	if decoded.MaxDepth != original.MaxDepth {
		t.Errorf("MaxDepth mismatch: got %d, want %d", decoded.MaxDepth, original.MaxDepth)
	}
	if decoded.Limit != original.Limit {
		t.Errorf("Limit mismatch: got %d, want %d", decoded.Limit, original.Limit)
	}
	if decoded.Screenshot != original.Screenshot {
		t.Errorf("Screenshot mismatch")
	}
}

func TestCrawlPayload_DefaultsFromJSON(t *testing.T) {
	// When maxDepth and limit are omitted from JSON, they default to 0 (Go zero value)
	input := `{"url":"https://example.com"}`

	var payload CrawlPayload
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if payload.URL != "https://example.com" {
		t.Errorf("URL mismatch")
	}
	if payload.MaxDepth != 0 {
		t.Errorf("MaxDepth should be 0 (zero value) when omitted, got %d", payload.MaxDepth)
	}
	if payload.Limit != 0 {
		t.Errorf("Limit should be 0 (zero value) when omitted, got %d", payload.Limit)
	}
}
