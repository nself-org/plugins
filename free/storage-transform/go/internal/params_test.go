package internal

import (
	"net/http"
	"net/url"
	"testing"
)

func makeRequest(t *testing.T, rawQuery string) *http.Request {
	t.Helper()
	u, err := url.Parse("http://localhost/storage/v1/object/public/bucket/img.jpg?" + rawQuery)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	r, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	return r
}

func TestParseParams_Defaults(t *testing.T) {
	r := makeRequest(t, "")
	p, err := ParseParams(r, 4000, 4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Quality != 85 {
		t.Errorf("default quality: got %d, want 85", p.Quality)
	}
	if p.Fit != FitCover {
		t.Errorf("default fit: got %s, want cover", p.Fit)
	}
	if p.DPR != 1.0 {
		t.Errorf("default DPR: got %f, want 1.0", p.DPR)
	}
	if p.Width != 0 || p.Height != 0 {
		t.Errorf("default dimensions: got w=%d h=%d, want 0 0", p.Width, p.Height)
	}
}

func TestParseParams_ValidWidth(t *testing.T) {
	r := makeRequest(t, "w=300")
	p, err := ParseParams(r, 4000, 4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Width != 300 {
		t.Errorf("got width %d, want 300", p.Width)
	}
}

func TestParseParams_ValidHeight(t *testing.T) {
	r := makeRequest(t, "h=200")
	p, err := ParseParams(r, 4000, 4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Height != 200 {
		t.Errorf("got height %d, want 200", p.Height)
	}
}

func TestParseParams_WidthExceedsMax(t *testing.T) {
	r := makeRequest(t, "w=99999")
	_, err := ParseParams(r, 4000, 4000)
	if err == nil {
		t.Error("expected error for w=99999, got nil")
	}
}

func TestParseParams_HeightExceedsMax(t *testing.T) {
	r := makeRequest(t, "h=99999")
	_, err := ParseParams(r, 4000, 4000)
	if err == nil {
		t.Error("expected error for h=99999, got nil")
	}
}

func TestParseParams_NegativeWidth(t *testing.T) {
	r := makeRequest(t, "w=-1")
	_, err := ParseParams(r, 4000, 4000)
	if err == nil {
		t.Error("expected error for w=-1, got nil")
	}
}

func TestParseParams_ZeroWidth(t *testing.T) {
	// w=0 is not a positive integer — should error.
	r := makeRequest(t, "w=0")
	_, err := ParseParams(r, 4000, 4000)
	if err == nil {
		t.Error("expected error for w=0, got nil")
	}
}

func TestParseParams_ValidQuality(t *testing.T) {
	r := makeRequest(t, "q=50")
	p, err := ParseParams(r, 4000, 4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Quality != 50 {
		t.Errorf("got quality %d, want 50", p.Quality)
	}
}

func TestParseParams_QualityOutOfRange(t *testing.T) {
	cases := []string{"q=0", "q=101", "q=-5"}
	for _, tc := range cases {
		r := makeRequest(t, tc)
		_, err := ParseParams(r, 4000, 4000)
		if err == nil {
			t.Errorf("expected error for %s, got nil", tc)
		}
	}
}

func TestParseParams_ValidFormats(t *testing.T) {
	cases := []struct {
		param    string
		expected OutputFormat
	}{
		{"fmt=webp", FormatWebP},
		{"fmt=jpeg", FormatJPEG},
		{"fmt=jpg", FormatJPEG},
		{"fmt=png", FormatPNG},
		{"fmt=avif", FormatAVIF},
	}
	for _, tc := range cases {
		r := makeRequest(t, tc.param)
		p, err := ParseParams(r, 4000, 4000)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.param, err)
			continue
		}
		if p.Format != tc.expected {
			t.Errorf("%s: got %s, want %s", tc.param, p.Format, tc.expected)
		}
	}
}

func TestParseParams_InvalidFormat(t *testing.T) {
	r := makeRequest(t, "fmt=bmp")
	_, err := ParseParams(r, 4000, 4000)
	if err == nil {
		t.Error("expected error for fmt=bmp, got nil")
	}
}

func TestParseParams_ValidFitModes(t *testing.T) {
	cases := []struct {
		param string
		fit   FitMode
	}{
		{"fit=cover", FitCover},
		{"fit=contain", FitContain},
		{"fit=fill", FitFill},
		{"fit=inside", FitInside},
		{"fit=outside", FitOutside},
	}
	for _, tc := range cases {
		r := makeRequest(t, tc.param)
		p, err := ParseParams(r, 4000, 4000)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.param, err)
			continue
		}
		if p.Fit != tc.fit {
			t.Errorf("%s: got %s, want %s", tc.param, p.Fit, tc.fit)
		}
	}
}

func TestParseParams_InvalidFit(t *testing.T) {
	r := makeRequest(t, "fit=stretch")
	_, err := ParseParams(r, 4000, 4000)
	if err == nil {
		t.Error("expected error for fit=stretch, got nil")
	}
}

func TestParseParams_DPRScalesWidth(t *testing.T) {
	// w=100, dpr=2.0 → width should become 200.
	r := makeRequest(t, "w=100&dpr=2.0")
	p, err := ParseParams(r, 4000, 4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Width != 200 {
		t.Errorf("got width %d after dpr=2.0, want 200", p.Width)
	}
}

func TestParseParams_DPRCapAtMax(t *testing.T) {
	// w=3000, dpr=2.0 → would be 6000, capped at maxW=4000.
	r := makeRequest(t, "w=3000&dpr=2.0")
	p, err := ParseParams(r, 4000, 4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Width != 4000 {
		t.Errorf("got width %d, want 4000 (capped)", p.Width)
	}
}

func TestParseParams_DPROutOfRange(t *testing.T) {
	cases := []string{"dpr=0.5", "dpr=4.0", "dpr=-1"}
	for _, tc := range cases {
		r := makeRequest(t, tc)
		_, err := ParseParams(r, 4000, 4000)
		if err == nil {
			t.Errorf("expected error for %s, got nil", tc)
		}
	}
}

func TestCacheKey_Stable(t *testing.T) {
	r1 := makeRequest(t, "w=300&h=200&q=80&fmt=webp&fit=cover")
	r2 := makeRequest(t, "h=200&w=300&fmt=webp&q=80&fit=cover")
	p1, _ := ParseParams(r1, 4000, 4000)
	p2, _ := ParseParams(r2, 4000, 4000)
	k1 := p1.CacheKey("bucket/img.jpg")
	k2 := p2.CacheKey("bucket/img.jpg")
	if k1 != k2 {
		t.Errorf("cache key not stable: %q vs %q", k1, k2)
	}
}

func TestCacheKey_DifferentPaths(t *testing.T) {
	r := makeRequest(t, "w=300")
	p, _ := ParseParams(r, 4000, 4000)
	k1 := p.CacheKey("bucket/img.jpg")
	k2 := p.CacheKey("bucket/other.jpg")
	if k1 == k2 {
		t.Error("cache keys should differ for different paths")
	}
}
