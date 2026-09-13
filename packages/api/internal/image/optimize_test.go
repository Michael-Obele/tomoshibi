package image

import (
	"bytes"
	stdimg "image"
	"image/png"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

func TestReencode_ResizeAndJPEG(t *testing.T) {
	// 400x300 source PNG.
	src := stdimg.NewRGBA(stdimg.Rect(0, 0, 400, 300))
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatalf("encode: %v", err)
	}

	out, mime, err := Reencode(in.Bytes(), domain.ImageProcessOptions{Format: "jpeg", MaxWidth: 200, Quality: 80})
	if err != nil {
		t.Fatalf("reencode: %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("expected jpeg mime, got %q", mime)
	}

	cfg, format, err := stdimg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if cfg.Width != 200 || cfg.Height != 150 {
		t.Errorf("expected 200x150, got %dx%d", cfg.Width, cfg.Height)
	}
	if format != "jpeg" {
		t.Errorf("expected jpeg format, got %q", format)
	}
}

func TestReencode_NoUpscale(t *testing.T) {
	src := stdimg.NewRGBA(stdimg.Rect(0, 0, 100, 50))
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatalf("encode: %v", err)
	}

	out, mime, err := Reencode(in.Bytes(), domain.ImageProcessOptions{Format: "png", MaxWidth: 1000})
	if err != nil {
		t.Fatalf("reencode: %v", err)
	}
	if mime != "image/png" {
		t.Errorf("expected png mime, got %q", mime)
	}
	cfg, _, err := stdimg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if cfg.Width != 100 || cfg.Height != 50 {
		t.Errorf("upscale must not happen: got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestReencode_InvalidFormat(t *testing.T) {
	src := stdimg.NewRGBA(stdimg.Rect(0, 0, 10, 10))
	var in bytes.Buffer
	_ = png.Encode(&in, src)

	if _, _, err := Reencode(in.Bytes(), domain.ImageProcessOptions{Format: "webp"}); err == nil {
		t.Error("expected error for webp output (no pure-Go encoder)")
	}
}

func TestReencode_GarbageInput(t *testing.T) {
	if _, _, err := Reencode([]byte("garbage"), domain.ImageProcessOptions{}); err == nil {
		t.Error("expected error for garbage input")
	}
}
