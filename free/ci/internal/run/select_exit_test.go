package run

// Purpose: D-1091 — a --job run exits with the result of the jobs it selected;
// the evidence stays fail-closed because jobs outside the selection did not run.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestJobSelectionExitFollowsSelectedJobs(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("true", "a"), "c": sh("true")}, gitFake{})
	r.C.Opts.Job = "b"
	res := r.run(t)
	if res.Exit != 0 {
		t.Fatalf("selected jobs passed but exit = %d\n%s", res.Exit, r.Out)
	}
	if res.Summary.Result != "fail" || res.Summary.Doc.Result != "fail" {
		t.Fatalf("evidence must stay fail-closed for a partial selection: %q / %q", res.Summary.Result, res.Summary.Doc.Result)
	}
	if !strings.Contains(strings.Join(res.Summary.Notes, "\n"), "--job") {
		t.Fatalf("no note explains the fail evidence: %v", res.Summary.Notes)
	}
}

func TestJobSelectionExitFailsWhenSelectedJobFails(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("false"), "b": sh("true", "a"), "c": sh("true")}, gitFake{})
	r.C.Opts.Job = "b"
	if res := r.run(t); res.Exit != 1 {
		t.Fatalf("a failed dependency of the selection must exit 1, got %d", res.Exit)
	}
}
