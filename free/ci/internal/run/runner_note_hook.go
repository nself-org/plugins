package run

// Purpose: the runner that actually ran each attempt, kept for the evidence. Every
// admission is noted by the run itself (CreateAttempt's caller), so a retried or
// confirmation attempt counts as much as the final one and nothing depends on the
// telemetry store. The dispatcher adds what it knows of the runner (platform,
// isolation) through NoteRunner, before it persists anything.
// Inputs:  NoteAdmission(run, job, n, admitted) per admitted attempt; NoteRunner(attempt,
// runner) from internal/schedtel; ForgetRun(run) when the run is concluded.
// Outputs: placedRunners(run): the per-attempt records and the run-level runner.
// Constraints: in-process and not persisted. A run whose admitted attempts all ran on one
// runner names it; any other run names the coordinator host (the producer) and lists every
// attempt. A dispatched attempt whose runner details are not known is written as unknown,
// never as local.

import (
	"runtime"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var placedRunnerNotes sync.Map // attempt id -> model.AttemptRunner

// NoteRunner records the details of the runner an attempt was admitted on. A runner with no node id is ignored.
func NoteRunner(attemptID string, r model.AttemptRunner) {
	if attemptID != "" && r.Node != "" {
		placedRunnerNotes.Store(attemptID, r)
	}
}

// ForgetRunner drops the details of a finished attempt.
func ForgetRunner(attemptID string) { placedRunnerNotes.Delete(attemptID) }

// RunnerNote returns the dispatcher's details of the runner an attempt was admitted on, false when none were noted.
func RunnerNote(attemptID string) (model.AttemptRunner, bool) {
	v, ok := placedRunnerNotes.Load(attemptID)
	if !ok {
		return model.AttemptRunner{}, false
	}
	return v.(model.AttemptRunner), true
}

// RunnerRefOf is the schema's run-level runner for an attempt runner.
func RunnerRefOf(r model.AttemptRunner) model.RunnerRef {
	return model.RunnerRef{ID: r.Node, Provider: r.Provider, Platform: r.Platform, Isolation: r.Isolation}
}

// CoordinatorRunner is the host that produced the document: where a job runs when nothing says otherwise.
func CoordinatorRunner() model.RunnerRef {
	return model.RunnerRef{ID: "local", Provider: "local", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"}
}

type admission struct {
	job       string
	n         int
	attemptID string
	node      string
	local     bool
}

type admissionLog struct {
	mu   sync.Mutex
	list []admission
}

var admissionLogs sync.Map // run id -> *admissionLog

// NoteAdmission records that an attempt of job was admitted on adm.RunnerID.
func NoteAdmission(runID, jobID string, n int, adm Admitted) {
	if runID == "" || adm.AttemptID == "" {
		return
	}
	v, _ := admissionLogs.LoadOrStore(runID, &admissionLog{})
	l := v.(*admissionLog)
	l.mu.Lock()
	l.list = append(l.list, admission{job: jobID, n: max(n, 1), attemptID: adm.AttemptID, node: adm.RunnerID, local: adm.Local})
	l.mu.Unlock()
}

// ForgetRun drops the admissions of a concluded run and the runner details of each of its attempts.
func ForgetRun(runID string) {
	if v, ok := admissionLogs.LoadAndDelete(runID); ok {
		l := v.(*admissionLog)
		l.mu.Lock()
		defer l.mu.Unlock()
		for _, a := range l.list {
			ForgetRunner(a.attemptID)
		}
	}
}

// runner is what the evidence says of where the attempt ran. The dispatcher's details win; an
// attempt admitted on this host without any is a plain local one; any other is a runner the
// coordinator knows only by id, and its other facts read unknown.
func (a admission) runner() (r model.AttemptRunner, plainLocal bool) {
	if r, ok := RunnerNote(a.attemptID); ok {
		return r, false
	}
	if a.local {
		c := CoordinatorRunner()
		return model.AttemptRunner{Kind: "local", Tier: "local", Node: a.node, Provider: c.Provider, Platform: c.Platform, Isolation: c.Isolation}, true
	}
	return model.AttemptRunner{Kind: "unknown", Tier: "unknown", Node: a.node, Provider: "unknown", Platform: "unknown", Isolation: "process"}, false
}

func admissionsOf(runID string) []admission {
	v, ok := admissionLogs.Load(runID)
	if !ok {
		return nil
	}
	l := v.(*admissionLog)
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]admission(nil), l.list...)
}

// AdmittedRunner is the runner of one attempt of a run, false when the run did not admit it.
func AdmittedRunner(runID, attemptID string) (model.AttemptRunner, bool) {
	for _, a := range admissionsOf(runID) {
		if a.attemptID == attemptID {
			r, _ := a.runner()
			return r, true
		}
	}
	return model.AttemptRunner{}, false
}

// placedRunners derives the attempt records and the run-level runner from every attempt the run admitted,
// retries and confirmations included. A run that only ever ran plain local attempts keeps no records.
func placedRunners(runID string) ([]model.AttemptEvidence, model.RunnerRef) {
	var attempts []model.AttemptEvidence
	byNode := map[string]model.RunnerRef{}
	allPlain := true
	for _, a := range admissionsOf(runID) {
		r, plain := a.runner()
		allPlain = allPlain && plain
		byNode[r.Node] = RunnerRefOf(r)
		attempts = append(attempts, model.AttemptEvidence{JobID: a.job, Attempt: a.n, Runner: r})
	}
	if allPlain {
		attempts = nil
	}
	if len(byNode) != 1 {
		return attempts, CoordinatorRunner()
	}
	for _, r := range byNode {
		return attempts, r
	}
	return attempts, CoordinatorRunner()
}
