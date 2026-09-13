package image

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEncodeToDataURI(t *testing.T) {
	p := NewProcessor()

	data := []byte("hello world")
	result := p.EncodeToDataURI(data, "text/plain")

	if !strings.HasPrefix(result, "data:text/plain;base64,") {
		t.Errorf("Expected data URI prefix, got %q", result[:40])
	}

	// Verify the base64 portion can be decoded back
	parts := strings.SplitN(result, ",", 2)
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("Failed to decode base64: %v", err)
	}

	if string(decoded) != "hello world" {
		t.Errorf("Decoded data mismatch: got %q", string(decoded))
	}
}

func TestEncodeToDataURI_ImageTypes(t *testing.T) {
	p := NewProcessor()

	tests := []struct {
		mimeType string
	}{
		{"image/png"},
		{"image/jpeg"},
		{"image/webp"},
		{"image/gif"},
		{"image/svg+xml"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			result := p.EncodeToDataURI([]byte{0x89, 0x50}, tt.mimeType)
			expected := fmt.Sprintf("data:%s;base64,", tt.mimeType)
			if !strings.HasPrefix(result, expected) {
				t.Errorf("Expected prefix %q, got %q", expected, result[:len(expected)+5])
			}
		})
	}
}

func TestDecodeDataURI(t *testing.T) {
	p := NewProcessor()
	original := []byte("test image data")
	mimeType := "image/png"

	dataURI := p.EncodeToDataURI(original, mimeType)

	decoded, decodedMime, err := DecodeDataURI(dataURI)
	if err != nil {
		t.Fatalf("DecodeDataURI failed: %v", err)
	}

	if decodedMime != mimeType {
		t.Errorf("MIME mismatch: got %q, want %q", decodedMime, mimeType)
	}

	if string(decoded) != string(original) {
		t.Errorf("Data mismatch: got %q, want %q", string(decoded), string(original))
	}
}

func TestDecodeDataURI_Invalid(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"Not a data URI", "https://example.com/image.png"},
		{"No comma", "data:image/png;base64"},
		{"Invalid base64", "data:image/png;base64,!!!invalid!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DecodeDataURI(tt.uri)
			if err == nil {
				t.Error("Expected error for invalid data URI")
			}
		})
	}
}

func TestFetchAndEncode(t *testing.T) {
	// Create a mock HTTP server that serves an image
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) // PNG header
	}))
	defer server.Close()

	p := NewProcessor()

	blob, err := p.FetchAndEncode(server.URL + "/image.png")
	if err != nil {
		t.Fatalf("FetchAndEncode failed: %v", err)
	}

	if blob.MimeType != "image/png" {
		t.Errorf("MIME type mismatch: got %q", blob.MimeType)
	}

	if !strings.HasPrefix(blob.DataURI, "data:image/png;base64,") {
		t.Errorf("DataURI prefix mismatch")
	}

	if len(blob.RawBytes) == 0 {
		t.Error("RawBytes should not be empty")
	}
}

func TestFetchAndEncode_TooLarge(t *testing.T) {
	// Serve a response larger than MaxImageSize
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		// Write MaxImageSize + 100 bytes
		data := make([]byte, MaxImageSize+100)
		w.Write(data)
	}))
	defer server.Close()

	p := NewProcessor()

	_, err := p.FetchAndEncode(server.URL + "/huge.png")
	if err == nil {
		t.Error("Expected error for oversized image")
	}

	if !strings.Contains(err.Error(), "size limit") {
		t.Errorf("Expected size limit error, got: %v", err)
	}
}

func TestFetchAndEncode_InvalidURL(t *testing.T) {
	p := NewProcessor()

	_, err := p.FetchAndEncode("ftp://example.com/image.png")
	if err == nil {
		t.Error("Expected error for invalid URL scheme")
	}
}

func TestFetchAndEncode_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewProcessor()

	_, err := p.FetchAndEncode(server.URL + "/missing.png")
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestFetchAndEncode_MimeTypeDetection(t *testing.T) {
	// Serve without Content-Type header — should auto-detect
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// PNG magic bytes
		w.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	}))
	defer server.Close()

	p := NewProcessor()

	blob, err := p.FetchAndEncode(server.URL + "/no-content-type")
	if err != nil {
		t.Fatalf("FetchAndEncode failed: %v", err)
	}

	if !strings.HasPrefix(blob.MimeType, "image/") {
		t.Errorf("Expected image MIME type, got %q", blob.MimeType)
	}
}

func TestIsValidImageURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com/image.png", true},
		{"http://example.com/image.jpg", true},
		{"ftp://example.com/image.png", false},
		{"data:image/png;base64,abc", false},
		{"/relative/path.png", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := IsValidImageURL(tt.url)
			if result != tt.expected {
				t.Errorf("IsValidImageURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestNewProcessor(t *testing.T) {
	p := NewProcessor()
	if p == nil {
		t.Fatal("NewProcessor should not return nil")
	}
	if p.client == nil {
		t.Fatal("Processor client should not be nil")
	}
}
