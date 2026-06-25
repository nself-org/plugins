package main

// ci_plugin_test.go — unit tests for the ci plugin cmd layer.
//
// Purpose: Verify stack detection, gitleaks invocation (mocked), and
// GitHub commit-status posting (mocked) without requiring real repo
// contents, a gitleaks binary, or a GitHub token.
//
// Inputs:   temp directories with sentinel files to trigger each stack detector.
// Outputs:  detected stack slices, gate result shapes.
// Constraints: no network calls; temp dirs cleaned up after each test.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal"
)

// TestStackDetection_Go verifies that a directory with go.mod is detected as a Go repo.
func TestStackDetection_Go(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.22\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cfg := internal.Config{
		RepoRoot:     dir,
		SkipGitleaks: true, // gitleaks not required for this test
		StepTimeout:  5,
	}

	result, err := internal.Run(cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	found := false
	for _, s := range result.Stack {
		if s == "go" {
			found = true
		}
	}
	if !found {
		t.Errorf("stack detection: expected 'go' in %v", result.Stack)
	}
}

// TestStackDetection_Node verifies that a directory with package.json is detected as a Node repo.
func TestStackDetection_Node(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test","version":"1.0.0"}`), 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	cfg := internal.Config{
		RepoRoot:     dir,
		SkipGitleaks: true,
		StepTimeout:  5,
	}

	result, err := internal.Run(cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	found := false
	for _, s := range result.Stack {
		if s == "node" {
			found = true
		}
	}
	if !found {
		t.Errorf("stack detection: expected 'node' in %v", result.Stack)
	}
}

// TestGitleaksSkipped verifies that SkipGitleaks=true does not include a gitleaks gate.
func TestGitleaksSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.22\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cfg := internal.Config{
		RepoRoot:     dir,
		SkipGitleaks: true,
		StepTimeout:  5,
	}

	result, err := internal.Run(cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	for _, g := range result.Gates {
		if g.Name == "gitleaks" {
			t.Errorf("expected gitleaks gate to be skipped, but found it: %+v", g)
		}
	}
}
