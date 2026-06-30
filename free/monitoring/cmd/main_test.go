package main

// main_test.go — tests for the monitoring config-render entrypoint.
//
// Purpose: Verify that render.RenderConfigs copies source configs to the output
//          directory and that prometheus.yml is present with the correct
//          scrape_interval, matching the known config template.
//
// Inputs:  temp dir containing a sample prometheus.yml.
// Outputs: output dir with prometheus.yml copied.
// Constraints: no network calls, no Docker daemon required.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/monitoring/internal/render"
)

// TestRenderConfigs_CopiesFiles verifies that render.RenderConfigs copies all
// files from the source directory tree to the output directory.
func TestRenderConfigs_CopiesFiles(t *testing.T) {
	// Build a minimal source tree with a prometheus.yml
	src := t.TempDir()
	promDir := filepath.Join(src, "prometheus")
	if err := os.MkdirAll(promDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	promYML := "global:\n  scrape_interval: 15s\n  evaluation_interval: 15s\n"
	if err := os.WriteFile(filepath.Join(promDir, "prometheus.yml"), []byte(promYML), 0644); err != nil {
		t.Fatalf("write prometheus.yml: %v", err)
	}

	out := t.TempDir()

	if err := render.RenderConfigs(src, out); err != nil {
		t.Fatalf("render.RenderConfigs() error: %v", err)
	}

	// prometheus.yml must exist in the output dir
	dstPath := filepath.Join(out, "prometheus", "prometheus.yml")
	content, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("prometheus.yml not in output: %v", err)
	}

	if !strings.Contains(string(content), "scrape_interval: 15s") {
		t.Errorf("prometheus.yml missing scrape_interval: %s", string(content))
	}
}

// TestRenderConfigs_MissingSource verifies that render.RenderConfigs fails
// gracefully when the source directory does not exist.
func TestRenderConfigs_MissingSource(t *testing.T) {
	out := t.TempDir()
	err := render.RenderConfigs("/nonexistent-monitoring-src-xyz987", out)
	if err == nil {
		t.Error("expected error for missing source dir, got nil")
	}
}
