package internal

import (
	"errors"
	"net/http"
	"strconv"
)

// FitMode controls how the image is cropped/padded to fit the target dimensions.
type FitMode string

const (
	FitCover   FitMode = "cover"
	FitContain FitMode = "contain"
	FitFill    FitMode = "fill"
	FitInside  FitMode = "inside"
	FitOutside FitMode = "outside"
)

// OutputFormat is the target image encoding.
type OutputFormat string

const (
	FormatWebP OutputFormat = "webp"
	FormatJPEG OutputFormat = "jpeg"
	FormatPNG  OutputFormat = "png"
	FormatAVIF OutputFormat = "avif"
	FormatSame OutputFormat = ""
)

// TransformParams holds validated transform parameters parsed from a request URL.
type TransformParams struct {
	Width   int          // target width px (0 = not set)
	Height  int          // target height px (0 = not set)
	Quality int          // encode quality 1-100
	Format  OutputFormat // output format (empty = same as source)
	Fit     FitMode      // crop/pad strategy
	DPR     float64      // device pixel ratio (multiplies w/h)
}

var validFormats = map[string]OutputFormat{
	"webp": FormatWebP,
	"jpeg": FormatJPEG,
	"jpg":  FormatJPEG,
	"png":  FormatPNG,
	"avif": FormatAVIF,
}

var validFits = map[string]FitMode{
	"cover":   FitCover,
	"contain": FitContain,
	"fill":    FitFill,
	"inside":  FitInside,
	"outside": FitOutside,
}

// ParseParams extracts and validates transform parameters from r.
// maxW / maxH are the server-side dimension caps.
func ParseParams(r *http.Request, maxW, maxH int) (TransformParams, error) {
	q := r.URL.Query()
	p := TransformParams{
		Quality: 85,
		Fit:     FitCover,
		DPR:     1.0,
	}

	if v := q.Get("w"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, errors.New("invalid w: must be a positive integer")
		}
		if n > maxW {
			return p, errors.New("w exceeds maximum allowed width")
		}
		p.Width = n
	}

	if v := q.Get("h"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, errors.New("invalid h: must be a positive integer")
		}
		if n > maxH {
			return p, errors.New("h exceeds maximum allowed height")
		}
		p.Height = n
	}

	if v := q.Get("q"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return p, errors.New("invalid q: must be 1-100")
		}
		p.Quality = n
	}

	if v := q.Get("fmt"); v != "" {
		f, ok := validFormats[v]
		if !ok {
			return p, errors.New("invalid fmt: must be webp, jpeg, png, or avif")
		}
		p.Format = f
	}

	if v := q.Get("fit"); v != "" {
		f, ok := validFits[v]
		if !ok {
			return p, errors.New("invalid fit: must be cover, contain, fill, inside, or outside")
		}
		p.Fit = f
	}

	if v := q.Get("dpr"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 1.0 || f > 3.0 {
			return p, errors.New("invalid dpr: must be 1.0-3.0")
		}
		p.DPR = f
	}

	// Apply DPR to dimensions.
	if p.DPR != 1.0 {
		if p.Width > 0 {
			w := int(float64(p.Width) * p.DPR)
			if w > maxW {
				w = maxW
			}
			p.Width = w
		}
		if p.Height > 0 {
			h := int(float64(p.Height) * p.DPR)
			if h > maxH {
				h = maxH
			}
			p.Height = h
		}
	}

	return p, nil
}

// CacheKey produces a stable, URL-safe cache key for a given image path + params.
func (p TransformParams) CacheKey(imagePath string) string {
	return imagePath +
		":w=" + strconv.Itoa(p.Width) +
		":h=" + strconv.Itoa(p.Height) +
		":q=" + strconv.Itoa(p.Quality) +
		":fmt=" + string(p.Format) +
		":fit=" + string(p.Fit)
}
