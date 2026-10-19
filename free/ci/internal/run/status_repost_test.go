package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// A run whose publication failed posts a red status; once `artifacts sign`
// publishes the release, RepostStatus posts the green one through the same
// outbox, and a newer seq supersedes the red status (D-1246).
func TestRepostStatusCorrectsAFailedPublicationStatus(t *testing.T) {
	t.Parallel()
	g := gitFake{}
	r := newRig(t, map[string]model.Job{"a": sh("true")}, g)
	r.C.Hooks.AfterRuns = []AfterRunHook{gateHook{func(RunInfo) error {
		return &publishFailure{err: errors.New("E717 signer_failed: run `nself ci artifacts sign x`")}
	}}}
	res := r.run(t)
	if posted := r.Rep.all(); res.Exit != 1 || len(posted) != 1 || posted[0].State != report.Failure {
		t.Fatalf("setup: exit %d, statuses %+v", res.Exit, posted)
	}
	line, err := RepostStatus(context.Background(), RepostRequest{Store: r.store(t), RunID: res.RunID, Root: r.C.Opts.Root, Reporter: r.Rep, Capture: g.capture})
	posted := r.Rep.all()
	if err != nil || len(posted) != 2 || posted[1].State != report.Success || posted[1].Revision != testSHA || !strings.Contains(line, "status posted (success)") {
		t.Fatalf("line %q err %v statuses %+v", line, err, posted)
	}
	if strings.Contains(posted[1].Description, "release not published") {
		t.Fatalf("the corrected status still carries the veto text: %q", posted[1].Description)
	}
}

// A run that did not pass, a dirty tree and an unknown run post nothing.
func TestRepostStatusPostsOnlyForAPassingBoundRun(t *testing.T) {
	t.Parallel()
	g := gitFake{}
	r := newRig(t, map[string]model.Job{"a": sh("false")}, g)
	res := r.run(t)
	before := len(r.Rep.all())
	req := RepostRequest{Store: r.store(t), RunID: res.RunID, Root: r.C.Opts.Root, Reporter: r.Rep, Capture: g.capture}
	if line, err := RepostStatus(context.Background(), req); err != nil || !strings.Contains(line, "status not posted") || len(r.Rep.all()) != before {
		t.Fatalf("failed run: %q %v %+v", line, err, r.Rep.all())
	}
	req.RunID = "no-such-run"
	if _, err := RepostStatus(context.Background(), req); err == nil {
		t.Fatal("an unknown run must be an error")
	}
	d := gitFake{dirty: true}
	r2 := newRig(t, map[string]model.Job{"a": sh("true")}, d)
	res2 := r2.run(t)
	line, err := RepostStatus(context.Background(), RepostRequest{Store: r2.store(t), RunID: res2.RunID, Root: r2.C.Opts.Root, Reporter: r2.Rep, Capture: d.capture})
	if err != nil || !strings.Contains(line, "status not posted") || len(r2.Rep.all()) != 0 {
		t.Fatalf("dirty run: %q %v %+v", line, err, r2.Rep.all())
	}
}
