package image

import (
	"fmt"
	stdimg "image"
	"io"

	// Register codecs for DecodeConfig.
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// maxSniffBytes caps how many bytes are read to decode image headers.
const maxSniffBytes = 512 << 10

// SniffDimensions reads just enough of an image stream to determine its
// dimensions and format without decoding the full image.
func SniffDimensions(r io.Reader) (width, height int, format string, err error) {
	cfg, format, err := stdimg.DecodeConfig(io.LimitReader(r, maxSniffBytes))
	if err != nil {
		return 0, 0, "", fmt.Errorf("decode config failed: %w", err)
	}
	return cfg.Width, cfg.Height, format, nil
}
