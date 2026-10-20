package runners_test

// Purpose: the pause-on-running acceptance test (review F6): a Job Hook that reports a job running pauses
// the runner of that job, and nothing else.
// Inputs:  Job Hook bodies (GitLab webhook docs shape) and fakeGL.
// Outputs: pass or fail.
// Constraints: no live network.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
)

const runningHook = `{"object_kind":"build","build_status":"running","build_id":4001,"project_id":4242,` +
	`"project":{"id":4242,"path_with_namespace":"acme/widgets"},"runner":{"id":%s,"description":"x","runner_type":"project_type"}}`

// TestPauseOnRunning proves acceptance 2 (the runner is paused when the running Job Hook arrives, the
// fixture records PUT paused=true, and the runner is deleted and absent afterwards): the pause happens only
// for a job the API confirms is running on that very runner, only for a runner of ours, once, and the
// payload alone (a forged hook) changes nothing.
func TestPauseOnRunning(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	r := f.registrar(clock)
	m, err := r.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	foreign := f.add("persistent-runner", false)
	body := func(id string) []byte { return []byte(sprintf(runningHook, id)) }

	// a forged hook for a job the API says is still pending: nothing is paused
	f.jobs[4001] = fakeJob{Ref: "main", Status: "pending"}
	ev, ok := runners.ParseJobHook(body(m.RunnerID))
	if !ok || ev.Project != testProject || ev.JobID != 4001 || itoa(ev.RunnerID) != m.RunnerID {
		t.Fatalf("parse: %+v %v", ev, ok)
	}
	if paused, err := r.OnRunning(ctx, ev); paused || err != nil || len(f.puts) != 0 {
		t.Fatalf("a pending job must not pause: paused %v err %v puts %v", paused, err, f.puts)
	}

	// the job runs on a different runner than the hook names: nothing is paused
	f.jobs[4001] = fakeJob{Ref: "main", Status: "running", RunnerID: foreign.ID}
	if paused, err := r.OnRunning(ctx, ev); paused || err != nil || len(f.puts) != 0 {
		t.Fatalf("a job on another runner must not pause: paused %v err %v puts %v", paused, err, f.puts)
	}

	// the job runs on our runner: paused, with exactly paused=true
	f.jobs[4001] = fakeJob{Ref: "main", Status: "running", RunnerID: itoa64(m.RunnerID)}
	paused, err := r.OnRunning(ctx, ev)
	if !paused || err != nil || len(f.puts) != 1 || f.puts[0]["paused"] != true || !f.runners[0].Paused {
		t.Fatalf("paused %v err %v puts %v", paused, err, f.puts)
	}
	if paused, err = r.OnRunning(ctx, ev); !paused || err != nil || len(f.puts) != 1 {
		t.Fatalf("a repeated hook must not pause twice: paused %v err %v puts %v", paused, err, f.puts)
	}

	// a runner that is not ours is never paused, even when the API says it runs the job
	fev := ev
	fev.RunnerID = foreign.ID
	f.jobs[4001] = fakeJob{Ref: "main", Status: "running", RunnerID: foreign.ID}
	if _, err = r.OnRunning(ctx, fev); code(err) != "E707" || len(f.puts) != 1 || f.runners[1].Paused {
		t.Fatalf("a foreign runner must be refused: %v puts %v", err, f.puts)
	}

	// the run closes: deleted and proven absent
	if err = r.Deregister(ctx, m.Ref()); err != nil {
		t.Fatal(err)
	}
	for _, x := range f.descriptions() {
		if x == m.Description {
			t.Fatalf("the paused runner must be absent after the run: %v", f.descriptions())
		}
	}
	if paused, err = r.OnRunning(ctx, ev); paused || err != nil {
		t.Fatalf("a hook for a runner that is gone is nothing to do: paused %v err %v", paused, err)
	}
}

// TestParseJobHook: only a running build with a project, a job and a runner is an event.
func TestParseJobHook(t *testing.T) {
	for name, body := range map[string]string{
		"pending":    `{"object_kind":"build","build_status":"pending","build_id":1,"project_id":2,"runner":{"id":3}}`,
		"no runner":  `{"object_kind":"build","build_status":"running","build_id":1,"project_id":2}`,
		"no job":     `{"object_kind":"build","build_status":"running","project_id":2,"runner":{"id":3}}`,
		"no project": `{"object_kind":"build","build_status":"running","build_id":1,"runner":{"id":3}}`,
		"push":       `{"object_kind":"push"}`,
		"not json":   `nope`,
	} {
		if ev, ok := runners.ParseJobHook([]byte(body)); ok {
			t.Errorf("%s must not be an event: %+v", name, ev)
		}
	}
	if ev, ok := runners.ParseJobHook([]byte(`{"object_kind":"build","build_status":"running","build_id":1,"project_id":2,"runner":{"id":3}}`)); !ok || ev.ProjectID != 2 || ev.Project != "" {
		t.Errorf("an older hook with only a project id: %+v %v", ev, ok)
	}
}
