package clientapi

// Purpose: the Backend of a running coordinator (P7-NODE-19): runs, logs and evidence read from the
// coordinator's own store and CI home, triggers stored through Coordinator.Enqueue.
// Inputs:  the store, the CI home (stored logs), an Enqueue function.
// Outputs: RunItem values whose project comes from the intake row that owns the run.
// Constraints: the user-facing run id is the pipeline id once the run has one and the intake id (`in-…`)
//          before; a pipeline no intake row claims has an empty project (admin only); in-flight runs are
//          tied to their intake row by revision, because the intake row names its pipeline only when the
//          dispatch finishes; no SQL here, only store methods; evidence is returned as stored.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// StoreBackend implements Backend over the coordinator's store.
type StoreBackend struct {
	Store   *store.Store
	Home    string // CI home: stored logs live under it
	Enqueue func(ctx context.Context, d coordinator.Delivery) (coordinator.Outcome, error)
}

var _ Backend = (*StoreBackend)(nil)

const intakePrefix = "in-"

// Submit stores a manual trigger. The coordinator's semantic key makes a repeat of the same revision a
// duplicate or a coalesced run, so a retried submit is safe.
func (b *StoreBackend) Submit(ctx context.Context, r SubmitRequest, _ string) (SubmitResult, error) {
	facts, _ := json.Marshal(map[string]string{"trigger": "api"})
	out, err := b.Enqueue(ctx, coordinator.Delivery{Source: "manual", Project: r.Project, Revision: r.Revision,
		TriggerClass: "change", Pipeline: r.Pipeline, Facts: facts})
	if err != nil {
		return SubmitResult{}, err
	}
	return SubmitResult{ID: out.ID, State: "queued", Outcome: out.Kind}, nil
}

// List returns intake-owned runs newest first, then pipelines no intake row owns.
func (b *StoreBackend) List(ctx context.Context, limit int) ([]RunItem, error) {
	rows, err := b.Store.IntakeList(ctx, limit)
	if err != nil {
		return nil, err
	}
	owned := map[string]bool{}
	out := make([]RunItem, 0, len(rows))
	for _, r := range rows {
		it, _, e := b.fromIntake(ctx, r, false)
		if e != nil {
			return nil, e
		}
		owned[it.ID] = true
		out = append(out, it)
	}
	page, err := b.Store.History(ctx, 0, limit)
	if err != nil {
		return nil, err
	}
	for _, p := range page.Pipelines {
		if !owned[p.ID] {
			out = append(out, RunItem{RunView: cirun.RunView{ID: p.ID, Revision: p.Revision, Status: p.Status}, Project: inflightProject(rows, p.Revision)})
		}
	}
	return out, nil
}

// Get resolves an intake id, a pipeline id, a unique pipeline id prefix, a full revision or "latest".
func (b *StoreBackend) Get(ctx context.Context, ref string) (RunItem, bool, error) {
	if strings.HasPrefix(ref, intakePrefix) {
		row, err := b.Store.IntakeGet(ctx, ref)
		if err != nil {
			var se *store.Error
			if errors.As(err, &se) && se.Code == "E605" {
				return RunItem{}, false, nil
			}
			return RunItem{}, false, err
		}
		return b.fromIntake(ctx, row, true)
	}
	if coordinator.ValidRevision(ref) { // `ci evidence <revision>`: the newest run of that commit
		rows, err := b.Store.PipelinesByRevision(ctx, ref)
		if err != nil || len(rows) == 0 {
			return RunItem{}, false, err
		}
		ref = rows[0].ID
	}
	row, err := cirun.ResolveRun(ctx, b.Store, ref)
	if err != nil {
		if cirun.CodeOf(err) == cirun.CodeRun {
			return RunItem{}, false, nil
		}
		return RunItem{}, false, err
	}
	v, err := cirun.ViewRun(ctx, b.Store, row)
	if err != nil {
		return RunItem{}, false, err
	}
	it := RunItem{RunView: v}
	if in, ok, e := b.Store.IntakeByPipeline(ctx, row.ID); e != nil {
		return RunItem{}, false, e
	} else if ok {
		it.Project = in.Project
	} else if !pipelineEnded(row.Status) { // still running: its intake row does not name it yet
		rows, e := b.Store.IntakeList(ctx, 200)
		if e != nil {
			return RunItem{}, false, e
		}
		it.Project = inflightProject(rows, row.Revision)
	}
	return it, true, nil
}

// inflightProject is the project of the dispatched intake rows for revision that do not name a pipeline
// yet; it is "" (an unowned run, admin only) when none matches or the matches disagree, so one commit
// reachable through two projects never lets either see the other's run.
func inflightProject(rows []store.IntakeRow, revision string) string {
	project := ""
	for _, r := range rows {
		if r.State != store.IntakeDispatched || r.PipelineID != "" || r.Revision != revision {
			continue
		}
		if project != "" && project != r.Project {
			return ""
		}
		project = r.Project
	}
	return project
}

// fromIntake describes the run an intake row stands for; jobs adds the job list of a started run.
func (b *StoreBackend) fromIntake(ctx context.Context, r store.IntakeRow, jobs bool) (RunItem, bool, error) {
	pid := r.PipelineID
	if pid == "" && r.State == store.IntakeDispatched {
		rows, err := b.Store.PipelinesByRevision(ctx, r.Revision)
		if err != nil {
			return RunItem{}, false, err
		}
		for _, p := range rows {
			if !pipelineEnded(p.Status) {
				pid = p.ID
				break
			}
		}
	}
	if pid != "" {
		if p, ok, err := b.Store.PipelineByID(ctx, pid); err != nil {
			return RunItem{}, false, err
		} else if ok {
			v := cirun.RunView{ID: p.ID, Revision: p.Revision, Status: p.Status}
			if jobs {
				if v, err = cirun.ViewRun(ctx, b.Store, p); err != nil {
					return RunItem{}, false, err
				}
			}
			return RunItem{RunView: v, Project: r.Project}, true, nil
		}
	}
	return RunItem{RunView: cirun.RunView{ID: r.ID, Revision: r.Revision, Status: intakeStatus(r)}, Project: r.Project}, true, nil
}

func intakeStatus(r store.IntakeRow) string {
	switch r.State {
	case store.IntakeQueued:
		return "queued"
	case store.IntakeDispatched:
		return "running"
	case store.IntakeFailed:
		return "failed"
	}
	return "lost" // done, but its pipeline is no longer in the store
}

func pipelineEnded(status string) bool {
	switch status {
	case "", "queued", "running":
		return false
	}
	return true
}

// Cancel asks every live attempt of a started run to stop.
func (b *StoreBackend) Cancel(ctx context.Context, ref, by string) (int, error) {
	if strings.HasPrefix(ref, intakePrefix) {
		return 0, Failf(ReasonConflict, "the run has not started yet; cancel it once it runs")
	}
	return cirun.CancelRun(ctx, b.Store, ref, "cancelled by "+by+" through the user API")
}

// Evidence returns the run-level evidence document as stored (signature included).
func (b *StoreBackend) Evidence(ctx context.Context, ref string) (json.RawMessage, error) {
	it, ok, err := b.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !ok || strings.HasPrefix(it.ID, intakePrefix) {
		return nil, Failf(ReasonNotFound, "the run has no evidence yet")
	}
	row, ok, err := b.Store.PipelineByID(ctx, it.ID)
	if err != nil || !ok {
		return nil, errors.Join(err, Failf(ReasonNotFound, "the run has no evidence yet"))
	}
	docs, err := evidence.ForRun(ctx, b.Store, row)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, Failf(ReasonNotFound, "the run has no evidence yet")
	}
	return json.RawMessage(docs[0].Body), nil
}

// Logs streams the stored log of a job's attempt.
func (b *StoreBackend) Logs(ctx context.Context, q LogQuery, fn func(logs.Line) error) (bool, error) {
	jobs, err := b.Store.PipelineJobs(ctx, q.Run)
	if err != nil {
		return false, err
	}
	for _, j := range jobs {
		if j.Name != q.Job && j.ID != q.Job {
			continue
		}
		atts, err := b.Store.AttemptsByJob(ctx, j.ID)
		if err != nil {
			return false, err
		}
		var pick *store.AttemptView
		for i := range atts {
			if (q.Attempt == 0 && (pick == nil || atts[i].N > pick.N)) || atts[i].N == q.Attempt {
				pick = &atts[i]
			}
		}
		if pick == nil {
			return false, nil
		}
		dir := logs.AttemptDir(b.Home, q.Run, pick.ID)
		if !logs.Exists(dir) && !q.Follow {
			return false, nil
		}
		if !q.Follow {
			return true, logs.Read(dir, q.After, fn)
		}
		ended := func() bool {
			st, _, e := b.Store.AttemptStateEpoch(ctx, pick.ID)
			return e != nil || st == "passed" || st == "failed" || st == "cancelled"
		}
		return true, logs.Follow(ctx, dir, q.After, ended, fn)
	}
	return false, nil
}
