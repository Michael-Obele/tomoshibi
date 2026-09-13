package image

import (
	"bytes"
	"image"
	stdjpeg "image/jpeg"
	stdpng "image/png"
	"testing"
)

func TestSniffDimensions_PNG(t *testing.T) {
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 320, 240))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	w, h, format, err := SniffDimensions(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("sniff failed: %v", err)
	}
	if w != 320 || h != 240 || format != "png" {
		t.Errorf("got %dx%d %q, want 320x240 png", w, h, format)
	}
}

func TestSniffDimensions_JPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 640, 480)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	w, h, format, err := SniffDimensions(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("sniff failed: %v", err)
	}
	if w != 640 || h != 480 || format != "jpeg" {
		t.Errorf("got %dx%d %q, want 640x480 jpeg", w, h, format)
	}
}

func TestSniffDimensions_Garbage(t *testing.T) {
	_, _, _, err := SniffDimensions(bytes.NewReader([]byte("not an image")))
	if err == nil {
		t.Fatal("expected error for garbage input")
	}
}
