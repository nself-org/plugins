package main

// Purpose: with --container the plan probes the pinned toolchain image's tool
// list, not the host PATH (D-1027). The image has no Swift (Epic D33), so a
// host that has swift on PATH must not get swift-build or swift-test placed.
// Constraints: PATH is a temp dir holding stub swift and docker executables, so
// the test does not depend on the host's tools.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func stubPath(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func planSwiftFixture(t *testing.T, container bool) *plan.Plan {
	t.Helper()
	stubPath(t, "swift", "docker")
	root := filepath.Join("..", "internal", "detect", "testdata", "swift", "swiftpm")
	o := RunOptions{Root: root, Container: container}
	p, _, _, err := preparePlan(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestContainerPlanIgnoresHostSwift(t *testing.T) {
	p := planSwiftFixture(t, true)
	for _, id := range []string{"swift-build", "swift-test"} {
		j := jobNamed(t, p, id)
		if j.State != plan.StateUnplaceable || j.Blocked != "unplaceable.tool" {
			t.Fatalf("%s with --container and swift only on the host PATH: %+v", id, j)
		}
	}
}

func TestHostPlanStillUsesHostSwift(t *testing.T) {
	p := planSwiftFixture(t, false)
	if j := jobNamed(t, p, "swift-build"); j.State != plan.StatePlanned {
		t.Fatalf("swift-build without --container and swift on PATH: %+v", j)
	}
}
