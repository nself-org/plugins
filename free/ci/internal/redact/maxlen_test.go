package redact

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestRegisterMaximum: a value over MaxValueLen is refused with E673 (never
// skipped) and the error hides the value; a value at the bound is accepted.
func TestRegisterMaximum(t *testing.T) {
	r := New()
	long := bytes.Repeat([]byte("x"), MaxValueLen+1)
	err := r.Register(long)
	var e *Error
	if !errors.As(err, &e) || e.Code != "E673" {
		t.Fatalf("want E673 for %d bytes, got %v", len(long), err)
	}
	if !strings.Contains(err.Error(), "maximum") || strings.Contains(err.Error(), "xxxxxxxx") {
		t.Fatalf("error text must name the maximum and hide the value: %q", err)
	}
	if err := r.Register(bytes.Repeat([]byte("y"), MaxValueLen)); err != nil {
		t.Fatalf("MaxValueLen bytes is allowed: %v", err)
	}
	// the refused value was not registered in any form
	if len(r.Scan(long[:64])) != 0 {
		t.Fatal("a refused value must not be registered")
	}
}

// TestFormExpansionBound: no form of a max-length value outgrows
// MaxFormExpansion times the value, so the 4 MiB scan overlap holds its longest
// form. The values are the adversarial ones: CRLF line ends (doubled, then
// escaped), non-ASCII runes, invalid UTF-8, and plain ASCII.
func TestFormExpansionBound(t *testing.T) {
	n := MaxValueLen
	values := map[string][]byte{
		"ascii":    bytes.Repeat([]byte("a"), n),
		"lf":       bytes.Repeat([]byte("\n"), n),
		"crlf":     bytes.Repeat([]byte("\r\n"), n/2),
		"bmp":      bytes.Repeat([]byte("é"), n/2),
		"astral":   bytes.Repeat([]byte("😀"), n/4),
		"invalid":  bytes.Repeat([]byte{0xff}, n),
		"space":    bytes.Repeat([]byte(" "), n),
		"reserved": bytes.Repeat([]byte("/+=&"), n/4),
	}
	for name, v := range values {
		a, b := valueForms(v)
		longest := 0
		for _, f := range append(a, b...) {
			longest = max(longest, len(f))
		}
		if limit := len(v) * MaxFormExpansion; longest > limit {
			t.Errorf("%s: longest form %d B exceeds %d x %d B = %d", name, longest, len(v), MaxFormExpansion, limit)
		}
	}
}
