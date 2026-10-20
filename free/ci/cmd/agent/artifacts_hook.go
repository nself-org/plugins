package main

// Purpose: deliver a lease's declared artifacts to the coordinator (P7-CACHE-04,
// Epic CACHE D6/D7). The executor removes the job workspace when the command
// ends, so the outputs are collected inside the executor's AfterJob callback
// (the only moment they exist) into a private store under the lease's job root;
// when the lease ends, this JobHook's After ships them over the carrier. The
// work itself is internal/artifacts/remote.Stage.
// Inputs:  the lease (declared and implicit artifacts), the pinned workspace of
// the finished job, the secret values the job received.
// Outputs: digest, chunk and artifact-ref frames; an infra failure of the
// attempt when a declared output cannot be delivered.
// Constraints: the hook acts only for a lease whose job declares outputs; only a
// passing job is collected and only a passing outcome is sent; collection runs
// in the agent process, never under the job's UID; the staging directory is
// removed when the lease ends.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/artifacts/remote"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func init() {
	must(session.RegisterJobHook(session.JobHook{Name: "artifacts", Before: artifactsBefore, After: artifactsAfter}))
}

// artifactStages maps an attempt id to the stage of its lease (spec.AttemptID in the executor, lease.AttemptID in hooks).
var artifactStages sync.Map

func artifactsBefore(_ context.Context, l protocol.Lease, root session.JobRoot) (session.JobSetup, error) {
	if !remote.Declares(l.Job) {
		return session.JobSetup{}, nil
	}
	st, err := remote.NewStage(filepath.Join(root.Dir, "artifacts-stage"), l.LeaseID)
	if err != nil {
		return session.JobSetup{}, err
	}
	artifactStages.Store(l.AttemptID, st)
	return session.JobSetup{}, nil
}

// artifactsAfterJob is the executor's AfterJob for a lease whose Before staged a store; nil otherwise.
func artifactsAfterJob(spec execSpec, in jobInputs) func(context.Context, *os.Root) error {
	v, ok := artifactStages.Load(spec.AttemptID)
	if !ok {
		return nil
	}
	st := v.(*remote.Stage)
	return func(ctx context.Context, workspace *os.Root) error {
		return st.Collect(ctx, workspace, spec.Job, secretScanner(in))
	}
}

// secretScanner knows every secret value the job received, so an artifact that carries one is refused.
func secretScanner(in jobInputs) *redact.Redactor {
	r := redact.New()
	for _, v := range in.Secrets {
		_ = r.Register([]byte(v)) // a value under the minimum length cannot be matched and is skipped
	}
	for _, v := range in.Masks {
		_ = r.Register([]byte(v))
	}
	return r
}

func artifactsAfter(ctx context.Context, l protocol.Lease, _ session.JobRoot, out session.Outcome, up session.Uploader) error {
	v, ok := artifactStages.LoadAndDelete(l.AttemptID)
	if !ok {
		return nil
	}
	st := v.(*remote.Stage)
	defer func() { _ = st.Close() }()
	if out.State != "passed" || st.Files() == 0 {
		return nil
	}
	src, ok := up.(session.OffsetSource)
	if !ok {
		return fmt.Errorf("artifacts: this carrier cannot receive offset answers")
	}
	return st.Ship(ctx, src.Offsets(), up)
}
