package trace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func resetTrace() {
	mu.Lock()
	defer mu.Unlock()
	enabled = false
	outputPath = ""
	warnedOnce = false
}

func TestTraceWritesSpansWhenEnvSet(t *testing.T) {
	resetTrace()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	configure(path)

	end := Start("test-span")
	time.Sleep(5 * time.Millisecond)
	end()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open trace file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected at least one span line")
	}
	line := scanner.Bytes()

	var span Span
	if err := json.Unmarshal(line, &span); err != nil {
		t.Fatalf("failed to unmarshal span: %v", err)
	}
	if span.Name != "test-span" {
		t.Errorf("expected span name 'test-span', got %q", span.Name)
	}
	if span.PID != os.Getpid() {
		t.Errorf("expected pid %d, got %d", os.Getpid(), span.PID)
	}
	if span.DurMS < 0 {
		t.Errorf("expected non-negative duration, got %d", span.DurMS)
	}

	resetTrace()
}

func TestTraceNoopWhenUnset(t *testing.T) {
	resetTrace()
	configure("")

	// Warm up so the compiler doesn't count init costs.
	end := Start("warmup")
	end()

	allocs := testing.AllocsPerRun(100, func() {
		end := Start("noop-span")
		end()
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocations when tracing is disabled, got %f", allocs)
	}

	resetTrace()
}

func TestTraceUnwritablePathDoesNotFail(t *testing.T) {
	resetTrace()

	var badPath string
	if runtime.GOOS == "windows" {
		badPath = `Z:\nonexistent\dir\trace.jsonl`
	} else {
		badPath = "/nonexistent-dir-xyz/trace.jsonl"
	}
	configure(badPath)

	// Must not panic or fail.
	end := Start("unwritable-span")
	end()

	resetTrace()
}

func TestTraceNestedSpansOrdered(t *testing.T) {
	resetTrace()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	configure(path)

	endOuter := Start("outer")
	time.Sleep(2 * time.Millisecond)
	endInner := Start("inner")
	time.Sleep(2 * time.Millisecond)
	endInner()
	endOuter()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open trace file: %v", err)
	}
	defer f.Close()

	var spans []Span
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var s Span
		if err := json.Unmarshal(scanner.Bytes(), &s); err != nil {
			t.Fatalf("failed to unmarshal span: %v", err)
		}
		spans = append(spans, s)
	}

	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Inner ends first, so it should be written first.
	if spans[0].Name != "inner" {
		t.Errorf("expected first written span to be 'inner', got %q", spans[0].Name)
	}
	if spans[1].Name != "outer" {
		t.Errorf("expected second written span to be 'outer', got %q", spans[1].Name)
	}

	// Outer started before inner.
	if spans[1].StartMS > spans[0].StartMS {
		t.Errorf("expected outer start_ms <= inner start_ms, got outer=%d inner=%d",
			spans[1].StartMS, spans[0].StartMS)
	}

	// Outer duration >= inner duration.
	if spans[1].DurMS < spans[0].DurMS {
		t.Errorf("expected outer dur_ms >= inner dur_ms, got outer=%d inner=%d",
			spans[1].DurMS, spans[0].DurMS)
	}

	resetTrace()
}
