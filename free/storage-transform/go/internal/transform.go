//go:build cgo

package internal

import (
	"fmt"
	"math"

	"github.com/davidbyttow/govips/v2/vips"
)

// Transformer wraps govips operations.
type Transformer struct{}

// NewTransformer initialises the govips library. Call Shutdown on exit.
func NewTransformer() *Transformer {
	vips.Startup(&vips.Config{
		ConcurrencyLevel: 0, // default = num CPUs
		MaxCacheFiles:    0,
		MaxCacheMem:      128 * 1024 * 1024, // 128 MB
		MaxCacheSize:     500,
		ReportLeaks:      false,
		CacheTrace:       false,
		CollectStats:     false,
	})
	return &Transformer{}
}

// Shutdown releases govips resources.
func (t *Transformer) Shutdown() {
	vips.Shutdown()
}

// Transform resizes and encodes srcBytes according to p.
// Returns the encoded bytes and the MIME type to use in the response.
func (t *Transformer) Transform(srcBytes []byte, p TransformParams) ([]byte, string, error) {
	img, err := vips.NewImageFromBuffer(srcBytes)
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	defer img.Close()

	// Compute target dimensions.
	targetW, targetH := computeDimensions(img.Width(), img.Height(), p)

	// Resize according to fit mode.
	if err := resize(img, targetW, targetH, p.Fit); err != nil {
		return nil, "", fmt.Errorf("resize: %w", err)
	}

	return encode(img, p)
}

func computeDimensions(srcW, srcH int, p TransformParams) (int, int) {
	w := p.Width
	h := p.Height
	if w == 0 && h == 0 {
		return srcW, srcH
	}
	if w == 0 {
		// Scale width proportionally.
		scale := float64(h) / float64(srcH)
		w = int(math.Round(float64(srcW) * scale))
	}
	if h == 0 {
		scale := float64(w) / float64(srcW)
		h = int(math.Round(float64(srcH) * scale))
	}
	return w, h
}

func resize(img *vips.ImageRef, targetW, targetH int, fit FitMode) error {
	srcW := img.Width()
	srcH := img.Height()

	switch fit {
	case FitCover:
		// Scale so the image covers the target box (no black bars), then crop centre.
		scaleW := float64(targetW) / float64(srcW)
		scaleH := float64(targetH) / float64(srcH)
		scale := math.Max(scaleW, scaleH)
		newW := int(math.Round(float64(srcW) * scale))
		newH := int(math.Round(float64(srcH) * scale))
		if err := img.Resize(scale, vips.KernelLanczos3); err != nil {
			return err
		}
		// Crop to target.
		left := (newW - targetW) / 2
		top := (newH - targetH) / 2
		return img.ExtractArea(left, top, targetW, targetH)

	case FitContain, FitInside:
		// Scale so the image fits within the target box, preserving aspect ratio.
		scaleW := float64(targetW) / float64(srcW)
		scaleH := float64(targetH) / float64(srcH)
		scale := math.Min(scaleW, scaleH)
		return img.Resize(scale, vips.KernelLanczos3)

	case FitFill:
		// Force exact target dimensions (distorts aspect ratio).
		scaleW := float64(targetW) / float64(srcW)
		if err := img.Resize(scaleW, vips.KernelLanczos3); err != nil {
			return err
		}
		// Thumbnail handles the second axis.
		return img.ThumbnailWithSize(targetW, targetH, vips.InterestingNone, vips.SizeBoth)

	case FitOutside:
		// Scale so the image covers at least one dimension fully.
		scaleW := float64(targetW) / float64(srcW)
		scaleH := float64(targetH) / float64(srcH)
		scale := math.Max(scaleW, scaleH)
		return img.Resize(scale, vips.KernelLanczos3)

	default:
		// Default to contain.
		scaleW := float64(targetW) / float64(srcW)
		scaleH := float64(targetH) / float64(srcH)
		scale := math.Min(scaleW, scaleH)
		return img.Resize(scale, vips.KernelLanczos3)
	}
}

func encode(img *vips.ImageRef, p TransformParams) ([]byte, string, error) {
	q := p.Quality

	switch p.Format {
	case FormatWebP:
		ep := vips.NewWebpExportParams()
		ep.Quality = q
		ep.Lossless = false
		buf, _, err := img.ExportWebp(ep)
		return buf, "image/webp", err

	case FormatAVIF:
		ep := vips.NewAvifExportParams()
		ep.Quality = q
		buf, _, err := img.ExportAvif(ep)
		return buf, "image/avif", err

	case FormatPNG:
		ep := vips.NewPngExportParams()
		ep.Compression = compressionFromQuality(q)
		buf, _, err := img.ExportPng(ep)
		return buf, "image/png", err

	case FormatJPEG:
		ep := vips.NewJpegExportParams()
		ep.Quality = q
		ep.StripMetadata = true
		buf, _, err := img.ExportJpeg(ep)
		return buf, "image/jpeg", err

	default:
		// Same as source format — use JPEG as a safe default for opaque images.
		ep := vips.NewJpegExportParams()
		ep.Quality = q
		ep.StripMetadata = true
		buf, _, err := img.ExportJpeg(ep)
		return buf, "image/jpeg", err
	}
}

// compressionFromQuality maps 1-100 quality to 0-9 PNG compression level.
func compressionFromQuality(q int) int {
	// Invert: high quality → low compression.
	level := 9 - int(math.Round(float64(q)/100.0*9.0))
	if level < 0 {
		level = 0
	}
	return level
}
