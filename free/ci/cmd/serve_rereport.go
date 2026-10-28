package main

// Purpose: the coordinator's Rereport hook (D-1320): a sweep found a commit without a terminal nself-ci
//          status although a run of it finished within the redelivery horizon; the stored result is sent
//          again instead of building the commit a second time.
// Inputs:  the finished intake row and the evidence the run recorded.
// Outputs: one status through the outbox (a new seq, so it replaces any parked older one).
// Constraints: only a bound, whole-gate GitHub run is re-sent (a partial run, a cancelled one or an
//          unbound one never posted a status on purpose); the state comes from the recorded result and the
//          description names the run, never evidence text the publication profile might filter.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func rereportDone(st *store.Store) func(context.Context, store.IntakeRow) error {
	return func(ctx context.Context, row store.IntakeRow) error {
		owner, repo, ok := strings.Cut(row.Project, "/")
		if !ok || row.Source != "github" || row.Pipeline != "" || row.PipelineID == "" {
			return nil
		}
		body, found, err := st.RunEvidence(ctx, row.PipelineID)
		if err != nil || !found {
			return err
		}
		var ev model.Evidence
		if err = json.Unmarshal(body, &ev); err != nil || ev.Binding.State != "bound" || ev.Revision != row.Revision {
			return nil
		}
		var state report.State
		switch ev.Result {
		case "pass":
			state = report.Success
		case "fail":
			state = report.Failure
		case "error":
			state = report.Error
		default:
			return nil
		}
		sender := report.Default
		if sender == nil || sender.Store != st {
			sender = &report.Sender{Store: st}
		}
		return sender.Repost(ctx, []report.Key{report.KeyFor(owner, repo, row.Revision)}, state, fmt.Sprintf("result of run %s sent again", row.PipelineID))
	}
}
