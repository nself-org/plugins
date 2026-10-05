package clui

import (
	"io"
	"os"
	"strings"
	"testing"
)

// capture runs fn with stdout (and stderr) redirected and returns both.
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

func TestPlainMessages(t *testing.T) {
	colorsEnabled = false
	out, err := capture(t, func() { Success("a"); Info("b"); Dimmed("c"); Warn("d"); Error("e") })
	if out != "✓ a\nℹ b\nc\n" || err != "⚠ d\n✗ e\n" {
		t.Fatalf("out %q err %q", out, err)
	}
}

func TestColouredMessages(t *testing.T) {
	colorsEnabled = true
	defer func() { colorsEnabled = false }()
	out, _ := capture(t, func() { Success("a"); Dimmed("c") })
	if !strings.Contains(out, "\033[0;32m✓\033[0m a") || !strings.Contains(out, "\033[2mc\033[0m") {
		t.Fatalf("out %q", out)
	}
}

func TestTable(t *testing.T) {
	out, _ := capture(t, func() {
		tb := NewTable("Name", "Path")
		tb.AddRow("a", "/x")
		tb.Render()
	})
	want := "┌──────┬──────┐\n│ Name │ Path │\n├──────┼──────┤\n│ a    │ /x   │\n└──────┴──────┘\n"
	if out != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}
}

func TestTerminalDetectionOnPipe(t *testing.T) {
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()
	if isTerminalFd(w.Fd()) {
		t.Fatal("a pipe is not a terminal")
	}
}
