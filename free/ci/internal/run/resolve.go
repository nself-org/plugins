package run

// Purpose: find a run in the store and describe it. `ci runs [run]` and
// `ci runs cancel <run>` (any process) read through here; RemoteResolvers are
// consulted first by the cmd layer, then the local store.
// Inputs:  the store and a run reference: "" or "latest" is the newest run,
// otherwise an id or a unique id prefix.
// Outputs: a pipeline row, or a RunView with its jobs.
// Constraints: read-only; an ambiguous or missing reference is E626.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ResolveRun finds the run ref names.
func ResolveRun(ctx context.Context, st *store.Store, ref string) (store.PipelineRow, error) {
	if ref != "" && ref != "latest" {
		if row, ok, err := st.PipelineByID(ctx, ref); err != nil || ok {
			return row, err
		}
	}
	page, err := st.History(ctx, 0, 100)
	if err != nil {
		return store.PipelineRow{}, err
	}
	if ref == "" || ref == "latest" {
		if len(page.Pipelines) == 0 {
			return store.PipelineRow{}, fail(CodeRun, "no runs recorded yet")
		}
		return page.Pipelines[0], nil
	}
	var found []store.PipelineRow
	for _, p := range page.Pipelines {
		if strings.HasPrefix(p.ID, ref) {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return store.PipelineRow{}, fail(CodeRun, "no run %q; list runs with `nself ci runs`", ref)
	}
	return store.PipelineRow{}, fail(CodeRun, "run reference %q matches %d runs; use more characters", ref, len(found))
}

// ViewRun describes a run and the state of each of its jobs.
func ViewRun(ctx context.Context, st *store.Store, row store.PipelineRow) (RunView, error) {
	jobs, err := st.PipelineJobs(ctx, row.ID)
	if err != nil {
		return RunView{}, err
	}
	attempts, err := st.PipelineAttempts(ctx, row.ID)
	if err != nil {
		return RunView{}, err
	}
	last := map[string]store.AttemptView{}
	for _, a := range attempts {
		if prev, ok := last[a.JobID]; !ok || a.N > prev.N {
			last[a.JobID] = a
		}
	}
	v := RunView{ID: row.ID, Revision: row.Revision, Status: row.Status}
	for _, j := range jobs {
		jv := JobView{Name: j.Name, State: j.Status}
		if a, ok := last[j.ID]; ok {
			jv.FailureClass = a.FailureClass
			if !terminal(j.Status) {
				jv.State = a.State
			}
		}
		v.Jobs = append(v.Jobs, jv)
	}
	return v, nil
}

func terminal(state string) bool {
	switch state {
	case "passed", "failed", "skipped", "cancelled":
		return true
	}
	return false
}

// ListRuns returns the newest runs first.
func ListRuns(ctx context.Context, st *store.Store, limit int) ([]store.PipelineRow, error) {
	page, err := st.History(ctx, 0, limit)
	return page.Pipelines, err
}
