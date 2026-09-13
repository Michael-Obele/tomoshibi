package image

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
)

const (
	// MaxImageSize is the maximum size of a single image to fetch (5MB).
	MaxImageSize = 5 * 1024 * 1024

	// FetchTimeout is the max time to wait for a single image download.
	FetchTimeout = 10 * time.Second

	// DefaultQuality is the default JPEG/WebP quality.
	DefaultQuality = 80
)

// Processor handles image encoding and optimization.
type Processor struct {
	client *http.Client
}

// NewProcessor creates a new image processor.
//
// The client refuses non-public destinations: image URLs come from scraped
// page markup, so a hostile page can point <img src> at an internal service
// and use the returned blob to exfiltrate it.
func NewProcessor() *Processor {
	return &Processor{
		client: safeurl.Client(FetchTimeout),
	}
}

// EncodeToDataURI converts raw bytes into a data URI string.
// Output: "data:image/png;base64,iVBORw0KGgo..."
func (p *Processor) EncodeToDataURI(data []byte, mimeType string) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
}

// DecodeDataURI extracts raw bytes and MIME type from a data URI.
func DecodeDataURI(dataURI string) ([]byte, string, error) {
	// Expected format: "data:image/png;base64,iVBORw0KGgo..."
	if !strings.HasPrefix(dataURI, "data:") {
		return nil, "", fmt.Errorf("not a data URI")
	}

	// Split on comma
	parts := strings.SplitN(dataURI, ",", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid data URI format")
	}

	// Extract MIME type from "data:image/png;base64"
	header := strings.TrimPrefix(parts[0], "data:")
	header = strings.TrimSuffix(header, ";base64")
	mimeType := header

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode failed: %w", err)
	}

	return data, mimeType, nil
}

// FetchAndEncode downloads an image URL and returns it as BlobData.
func (p *Processor) FetchAndEncode(imageURL string) (*domain.BlobData, error) {
	return p.FetchAndEncodeLimit(imageURL, MaxImageSize)
}

// FetchAndEncodeLimit downloads an image URL with a caller-supplied size cap
// in bytes (<= 0 falls back to MaxImageSize) and returns it as BlobData.
func (p *Processor) FetchAndEncodeLimit(imageURL string, maxBytes int64) (*domain.BlobData, error) {
	if !IsValidImageURL(imageURL) {
		return nil, fmt.Errorf("invalid image URL: %s", imageURL)
	}
	if maxBytes <= 0 {
		maxBytes = MaxImageSize
	}

	resp, err := p.client.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch returned status %d", resp.StatusCode)
	}

	// Enforce size limit via LimitReader
	limitedReader := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image exceeded size limit (%d bytes max)", maxBytes)
	}

	// Detect MIME type
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
		mimeType = http.DetectContentType(data)
	}
	// Strip charset suffix if present: "image/jpeg; charset=utf-8" → "image/jpeg"
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	return &domain.BlobData{
		DataURI:  p.EncodeToDataURI(data, mimeType),
		MimeType: mimeType,
		RawBytes: data,
	}, nil
}

// IsValidImageURL checks if a URL is a valid http/https URL.
func IsValidImageURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://")
}
