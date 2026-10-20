package runners

// Purpose: Pause and OnRunning, the review F6 defence: a runner is paused as soon as a Job Hook reports its
// job running, so a token that job reads (the shell executor runs the job on the same user as the runner)
// cannot take a second job. The runner is deleted when the run closes (runnerjob.Finalize).
// Inputs:  a Job Hook body (ParseJobHook) or a RunningEvent; the numeric runner id.
// Outputs: PUT /runners/:id {"paused": true}; OnRunning reports whether it paused one.
// Constraints: nothing in the event is trusted. OnRunning reads the job from the API and requires it to be
//          running on the runner the event names, and Pause reads the runner and requires the nself-glr-
//          description, so a forged hook can at most pause a runner of ours that is running that very job.
//          Both are idempotent (a 404 means the runner is already gone). The Job Hook source (HOST-21)
//          skips jobs that are not pending, so the ingress calls OnRunning from the coordinator wiring
//          (P7-NODE-20); the hook source is not changed here.

import (
	"context"
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
)

// RunningEvent is what a "job is running" Job Hook names.
type RunningEvent struct {
	Project   string // group/project, may be empty
	ProjectID int64
	JobID     int64
	RunnerID  int64
}

// ParseJobHook reads a Job Hook body; ok is true only for a running job with a project, a job and a runner.
func ParseJobHook(body []byte) (RunningEvent, bool) {
	var p struct {
		ObjectKind  string `json:"object_kind"`
		BuildStatus string `json:"build_status"`
		BuildID     int64  `json:"build_id"`
		ProjectID   int64  `json:"project_id"`
		Project     *struct {
			ID   int64  `json:"id"`
			Path string `json:"path_with_namespace"`
		} `json:"project"`
		Runner *struct {
			ID int64 `json:"id"`
		} `json:"runner"`
	}
	if json.Unmarshal(body, &p) != nil || p.ObjectKind != "build" || p.BuildStatus != "running" || p.BuildID <= 0 || p.Runner == nil || p.Runner.ID <= 0 {
		return RunningEvent{}, false
	}
	ev := RunningEvent{ProjectID: p.ProjectID, JobID: p.BuildID, RunnerID: p.Runner.ID}
	if p.Project != nil {
		ev.Project = p.Project.Path
		if ev.ProjectID == 0 {
			ev.ProjectID = p.Project.ID
		}
	}
	return ev, ev.Project != "" || ev.ProjectID > 0
}

// Pause stops the runner from taking more jobs. A runner that is gone is nothing to do.
func (r *Registrar) Pause(ctx context.Context, ref providers.RunnerRef) error {
	_, err := r.pause(ctx, ref)
	return err
}

// pause reports whether the runner is one of ours and now paused (found false: gone).
func (r *Registrar) pause(ctx context.Context, ref providers.RunnerRef) (bool, error) {
	v, found, err := r.ours(ctx, "pause", ref)
	if err != nil || !found {
		return false, err
	}
	if v.Paused {
		return true, nil
	}
	resp, err := r.c.Do(ctx, "PUT", runnerPath, map[string]string{"runner": ref.ID}, map[string]bool{"paused": true}, nil)
	if err != nil {
		if resp.Status == 404 {
			return false, nil
		}
		return false, runnerErr(err, "pause")
	}
	return true, nil
}

// OnRunning pauses the runner of a job that has started; paused is false when the event does not describe
// a running job on a runner of ours (nothing is changed then).
func (r *Registrar) OnRunning(ctx context.Context, ev RunningEvent) (paused bool, err error) {
	p, ok := api.ParseProject(ev.Project)
	if !ok {
		if ev.ProjectID <= 0 {
			return false, e702("scope.repo")
		}
		p = api.Project{Path: itoa(ev.ProjectID)}
	}
	if ev.JobID <= 0 || ev.RunnerID <= 0 {
		return false, e702("runner.id")
	}
	var job jobView
	if _, err = r.c.Do(ctx, "GET", jobPath, p.With("job", itoa(ev.JobID)), nil, &job); err != nil {
		if api.IsStatus(err, 404) {
			return false, nil
		}
		return false, runnerErr(err, "pause")
	}
	if job.Status != "running" || job.Runner == nil || job.Runner.ID != ev.RunnerID {
		return false, nil
	}
	return r.pause(ctx, providers.RunnerRef{ID: itoa(ev.RunnerID), Scope: providers.RunnerScope{Repo: ev.Project}})
}
