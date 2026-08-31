package internal

import (
	"testing"
)

// TestTransformParams_QualityStoredCorrectly confirms quality values are stored as-is.
func TestTransformParams_QualityStoredCorrectly(t *testing.T) {
	cases := []int{1, 50, 85, 100}
	for _, q := range cases {
		tp := TransformParams{Quality: q}
		if tp.Quality != q {
			t.Errorf("quality %d: got %d", q, tp.Quality)
		}
	}
}

func TestTransformParams_CacheKeyIncludesAllParams(t *testing.T) {
	a := TransformParams{Width: 300, Height: 200, Quality: 80, Format: FormatWebP, Fit: FitCover}
	b := TransformParams{Width: 300, Height: 200, Quality: 80, Format: FormatAVIF, Fit: FitCover}
	path := "bucket/photo.jpg"
	ka := a.CacheKey(path)
	kb := b.CacheKey(path)
	if ka == kb {
		t.Error("cache keys should differ when format differs")
	}
}

func TestTransformParams_CacheKeyFitDifference(t *testing.T) {
	a := TransformParams{Width: 300, Height: 200, Quality: 80, Fit: FitCover}
	b := TransformParams{Width: 300, Height: 200, Quality: 80, Fit: FitContain}
	ka := a.CacheKey("img.jpg")
	kb := b.CacheKey("img.jpg")
	if ka == kb {
		t.Error("cache keys should differ when fit differs")
	}
}

func TestTransformParams_CacheKeyWidthDifference(t *testing.T) {
	a := TransformParams{Width: 100, Quality: 85, Fit: FitCover}
	b := TransformParams{Width: 200, Quality: 85, Fit: FitCover}
	ka := a.CacheKey("img.jpg")
	kb := b.CacheKey("img.jpg")
	if ka == kb {
		t.Error("cache keys should differ when width differs")
	}
}

func TestFitModeConstants(t *testing.T) {
	modes := []struct {
		mode FitMode
		str  string
	}{
		{FitCover, "cover"},
		{FitContain, "contain"},
		{FitFill, "fill"},
		{FitInside, "inside"},
		{FitOutside, "outside"},
	}
	seen := map[string]bool{}
	for _, m := range modes {
		s := string(m.mode)
		if s != m.str {
			t.Errorf("FitMode %v: got string %q, want %q", m.mode, s, m.str)
		}
		if seen[s] {
			t.Errorf("duplicate FitMode value: %q", s)
		}
		seen[s] = true
	}
}

func TestOutputFormatConstants(t *testing.T) {
	formats := []struct {
		f   OutputFormat
		str string
	}{
		{FormatWebP, "webp"},
		{FormatJPEG, "jpeg"},
		{FormatPNG, "png"},
		{FormatAVIF, "avif"},
	}
	seen := map[string]bool{}
	for _, f := range formats {
		s := string(f.f)
		if s != f.str {
			t.Errorf("OutputFormat %v: got string %q, want %q", f.f, s, f.str)
		}
		if seen[s] {
			t.Errorf("duplicate OutputFormat value: %q", s)
		}
		seen[s] = true
	}
}
