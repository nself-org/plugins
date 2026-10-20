package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// D-1270: the config digest a bare run stores on the pipeline row is the one the
// release freshness gate compares (attest.ConfigDigest of the parsed config, as
// evidence.InputsAt computes it), not the sha256 of the file bytes.
func TestPreparePlanDigestMatchesEvidenceInputs(t *testing.T) {
	r := newEvRepo(t)
	bare.cfg = ""
	t.Cleanup(func() { bare.cfg = "" })
	ctx := context.Background()
	want, err := evidence.InputsAt(ctx, report.Capture, r.dir, r.head())
	if err != nil {
		t.Fatal(err)
	}
	_, got, _, err := preparePlan(ctx, RunOptions{Root: r.dir})
	if err != nil {
		t.Fatal(err)
	}
	if got != want.Config {
		t.Errorf("committed config: digest %s, the release gate expects %s", got, want.Config)
	}

	if err = os.Remove(filepath.Join(r.dir, ".nself", "ci.yaml")); err != nil {
		t.Fatal(err)
	}
	git(t, r.dir, "add", "-A")
	git(t, r.dir, "commit", "-q", "-m", "drop the config")
	if want, err = evidence.InputsAt(ctx, report.Capture, r.dir, r.head()); err != nil {
		t.Fatal(err)
	}
	if _, got, _, err = preparePlan(ctx, RunOptions{Root: r.dir}); err != nil {
		t.Fatal(err)
	}
	if got != want.Config {
		t.Errorf("built-in defaults: digest %s, the release gate expects %s", got, want.Config)
	}
}
