package run

// Purpose: correct the `nself-ci` commit status after a recovery. A run whose
// release could not be published posts a failure (finish.go); once
// `nself ci artifacts sign <run>` publishes the release, RepostStatus posts the
// run's own status again (D-1246).
// Inputs:  a RepostRequest: the run store, the run id and the project root.
// Outputs: the one status line, as the run prints it.
// Constraints: goes through prepareStatus and postFinal, the seam finish.go
// uses, so it takes a newer outbox seq and the registered Publisher and gates
// apply; only a passing run whose evidence is bound to the commit posts (a
// failed run keeps its status); the caller says when the release is published.

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// RepostRequest names the run to post again.
type RepostRequest struct {
	Store *store.Store
	RunID string
	// Root is the project root ("" = the run row's project).
	Root string
	// Reporter and Capture are for tests; nil resolve the GitHub remote and gh auth.
	Reporter report.Reporter
	Capture  Capture
}

// RepostStatus posts the status of a finished run again and returns the line
// that says what happened. An error means the run or its evidence is unknown.
func RepostStatus(ctx context.Context, r RepostRequest) (string, error) {
	pipe, ok, err := r.Store.PipelineByID(ctx, r.RunID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("run " + r.RunID + " is unknown")
	}
	ev, digest, err := storedEvidence(ctx, r.Store, pipe)
	if err != nil {
		return "", err
	}
	switch {
	case ev.Result != "pass":
		return notPosted("run " + r.RunID + " did not pass"), nil
	case ev.Binding.State != "bound" || pipe.Revision == "":
		return notPosted(report.ReasonUnbound), nil
	}
	root := firstNonEmpty(r.Root, pipe.Project)
	c := &Coordinator{Opts: Options{Root: root}, Reporter: r.Reporter, Capture: r.Capture}
	p := c.prepareStatus(ctx, r.Store, Revision{SHA: pipe.Revision})
	if p.Reporter == nil || p.Outbox == nil {
		return notPosted(firstNonEmpty(p.Reason, report.ReasonNoRemote)), nil
	}
	info := RunInfo{RunID: r.RunID, Revision: pipe.Revision, EvidenceDigest: digest, Result: ev.Result, Evidence: ev}
	line, _ := postFinal(ctx, Hooks{}.Resolved(), p, pipe.Revision, "", info, ev)
	return line, nil
}

// storedEvidence reads the newest evidence the run recorded.
func storedEvidence(ctx context.Context, st *store.Store, pipe store.PipelineRow) (model.Evidence, string, error) {
	rows, err := st.EvidenceByRevision(ctx, pipe.Revision, pipe.InputDigest)
	if err != nil {
		return model.Evidence{}, "", err
	}
	for _, row := range rows { // newest first
		if row.PipelineID != pipe.ID {
			continue
		}
		var ev model.Evidence
		if err = json.Unmarshal(row.Body, &ev); err != nil {
			return model.Evidence{}, "", errors.New("the stored evidence of run " + pipe.ID + " cannot be read")
		}
		digest, err := evidenceDigest(ev)
		return ev, digest, err
	}
	return model.Evidence{}, "", errors.New("run " + pipe.ID + " has no stored evidence")
}
