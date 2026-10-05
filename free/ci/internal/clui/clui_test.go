package clui

import (
	"io"
	"os"
	"testing"
)

// capture runs fn with stdout and stderr redirected and returns both.
func capture(t *testing.T, fn func()) (string, string) {
	t.Helper()
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we
	fn()
	os.Stdout, os.Stderr = so, se
	_ = wo.Close()
	_ = we.Close()
	bo, _ := io.ReadAll(ro)
	be, _ := io.ReadAll(re)
	return string(bo), string(be)
}

// TestPlainFraming pins the exact bytes core prints on a non-terminal.
func TestPlainFraming(t *testing.T) {
	colorsEnabled = false
	out, errOut := capture(t, func() {
		Section("nself-ci gate")
		Info("repo: /x")
		Success("ok")
		Warn("careful")
		Error("bad")
	})
	if want := "\n→ nself-ci gate\nℹ repo: /x\n✓ ok\n"; out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
	if want := "⚠ careful\n✗ bad\n"; errOut != want {
		t.Fatalf("stderr = %q, want %q", errOut, want)
	}
}

// TestColourWraps proves colour codes appear only when enabled.
func TestColourWraps(t *testing.T) {
	colorsEnabled = true
	defer func() { colorsEnabled = false }()
	if got := C(blue, "x"); got != blue+"x"+reset {
		t.Fatalf("C = %q", got)
	}
	colorsEnabled = false
	if got := C(blue, "x"); got != "x" {
		t.Fatalf("C plain = %q", got)
	}
}
