package image

import (
	"bytes"
	"fmt"
	stdimg "image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// Reencode decodes an image, optionally downsizes it to MaxWidth
// (preserving aspect ratio, never upscaling), and re-encodes it as
// "jpeg" (default) or "png" at the requested quality (0 → DefaultQuality).
// WebP output is intentionally unsupported: there is no pure-Go webp
// encoder, and cgo bindings conflict with hobby-tier container builds.
func Reencode(data []byte, opts domain.ImageProcessOptions) ([]byte, string, error) {
	src, _, err := stdimg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode failed: %w", err)
	}

	out := src
	bounds := src.Bounds()
	if opts.MaxWidth > 0 && bounds.Dx() > opts.MaxWidth {
		height := bounds.Dy() * opts.MaxWidth / bounds.Dx()
		dst := stdimg.NewRGBA(stdimg.Rect(0, 0, opts.MaxWidth, height))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
		out = dst
	}

	quality := opts.Quality
	if quality <= 0 || quality > 100 {
		quality = DefaultQuality
	}

	var buf bytes.Buffer
	switch opts.Format {
	case "png":
		if err := png.Encode(&buf, out); err != nil {
			return nil, "", fmt.Errorf("png encode failed: %w", err)
		}
		return buf.Bytes(), "image/png", nil
	case "jpeg", "jpg", "":
		if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", fmt.Errorf("jpeg encode failed: %w", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	default:
		return nil, "", fmt.Errorf("unsupported output format %q (use jpeg or png)", opts.Format)
	}
}
