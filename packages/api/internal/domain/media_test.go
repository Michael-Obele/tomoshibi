package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestImageData_JSONOmitEmpty(t *testing.T) {
	// Only URL set — all optional fields should be omitted
	img := ImageData{
		URL: "https://example.com/image.jpg",
	}

	data, err := json.Marshal(img)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	required := []string{"url"}
	for _, key := range required {
		if _, exists := raw[key]; !exists {
			t.Errorf("Expected %q to be present", key)
		}
	}

	optional := []string{"blob", "alt", "title", "width", "height", "format", "size_bytes", "source"}
	for _, key := range optional {
		if _, exists := raw[key]; exists {
			t.Errorf("Expected %q to be omitted when empty", key)
		}
	}
}

func TestImageData_FullPopulation(t *testing.T) {
	img := ImageData{
		URL:        "https://example.com/photo.webp",
		Blob:       "data:image/webp;base64,UklGR...",
		Alt:        "A photo",
		Title:      "Photo Title",
		Width:      1920,
		Height:     1080,
		Format:     "webp",
		SizeBytes:  125000,
		SourceType: "content",
	}

	data, err := json.Marshal(img)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ImageData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.URL != img.URL {
		t.Errorf("URL mismatch")
	}
	if decoded.Blob != img.Blob {
		t.Errorf("Blob mismatch")
	}
	if decoded.Width != 1920 || decoded.Height != 1080 {
		t.Errorf("Dimensions mismatch: %dx%d", decoded.Width, decoded.Height)
	}
}

func TestScreenshotData_JSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	ss := ScreenshotData{
		Blob:       "data:image/png;base64,iVBOR...",
		Format:     "png",
		Width:      1280,
		Height:     800,
		FullPage:   false,
		SizeBytes:  42000,
		CapturedAt: now,
	}

	data, err := json.Marshal(ss)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ScreenshotData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Width != 1280 || decoded.Height != 800 {
		t.Errorf("Dimensions mismatch")
	}
	if decoded.Format != "png" {
		t.Errorf("Format mismatch: %q", decoded.Format)
	}
}

func TestScreenshotData_OmitEmptyBlob(t *testing.T) {
	// URL-only mode — blob should be omitted
	ss := ScreenshotData{
		URL:    "https://storage.example.com/screenshot.png",
		Format: "png",
		Width:  1280,
		Height: 800,
	}

	data, _ := json.Marshal(ss)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, exists := raw["blob"]; exists {
		t.Error("Blob should be omitted when empty")
	}
	if _, exists := raw["url"]; !exists {
		t.Error("URL should be present")
	}
}

func TestBlobData_RawBytesNotSerialized(t *testing.T) {
	blob := BlobData{
		DataURI:  "data:image/png;base64,abc",
		MimeType: "image/png",
		RawBytes: []byte{0x89, 0x50, 0x4E, 0x47},
	}

	data, _ := json.Marshal(blob)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// RawBytes has json:"-" so it should never appear in JSON
	for key := range raw {
		if key == "raw_bytes" || key == "RawBytes" {
			t.Error("RawBytes should not be serialized to JSON")
		}
	}
}

func TestScrapeResult_WithImages(t *testing.T) {
	result := ScrapeResult{
		URL:      "https://example.com",
		Markdown: "# Example",
		Screenshot: &ScreenshotData{
			Blob:   "data:image/png;base64,abc",
			Format: "png",
			Width:  1280,
			Height: 800,
		},
		Images: []ImageData{
			{
				URL:        "https://example.com/hero.jpg",
				Blob:       "data:image/jpeg;base64,def",
				SourceType: "og:image",
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ScrapeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Screenshot == nil {
		t.Fatal("Screenshot should not be nil")
	}
	if decoded.Screenshot.Format != "png" {
		t.Errorf("Screenshot format mismatch")
	}
	if len(decoded.Images) != 1 {
		t.Fatalf("Expected 1 image, got %d", len(decoded.Images))
	}
	if decoded.Images[0].SourceType != "og:image" {
		t.Errorf("Image source mismatch")
	}
}

func TestScrapeResult_WithoutImages_BackwardCompat(t *testing.T) {
	// Old-style result without images — new fields should be omitted
	result := ScrapeResult{
		URL:      "https://example.com",
		Markdown: "# Example",
	}

	data, _ := json.Marshal(result)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, exists := raw["screenshot"]; exists {
		t.Error("Screenshot should be omitted when nil")
	}
	if _, exists := raw["images"]; exists {
		t.Error("Images should be omitted when nil")
	}
}

func TestImageTransportFormat_Constants(t *testing.T) {
	if ImageFormatURL != "url" {
		t.Errorf("ImageFormatURL should be 'url', got %q", ImageFormatURL)
	}
	if ImageFormatBlob != "blob" {
		t.Errorf("ImageFormatBlob should be 'blob', got %q", ImageFormatBlob)
	}
}

func TestScrapeOptions_Defaults(t *testing.T) {
	opts := ScrapeOptions{}

	if opts.Screenshot != false {
		t.Error("Screenshot should default to false")
	}
	if opts.Images != false {
		t.Error("Images should default to false")
	}
	if opts.ImageFormat != "" {
		t.Error("ImageFormat should default to empty")
	}
}
