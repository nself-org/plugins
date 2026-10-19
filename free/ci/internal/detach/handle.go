package detach

// Purpose: contract:ci.run-handle v1 (experimental until P8, Epic D30): the
// document `nself ci --detach` prints once the run is queued.
// Inputs:  run id, pid, coordinator target and stream file of a detached run.
// Outputs: Handle, whose commands are argv arrays (never shell strings).
// Constraints: additive fields only within v1; the wait and follow commands
// carry --coordinator when the run lives on a coordinator (the other three
// resolve the run themselves; logs names one job); pid and stream_path always name the LOCAL
// detached process and its event file, also with --coordinator.

// Schema is the handle's contract id.
const Schema = "ci.run-handle/v1"

// StateQueued is the only state a handle is issued in.
const StateQueued = "queued"

// Local is the coordinator value of a run on this machine.
const Local = "local"

// Commands are argv arrays that read or stop the run.
type Commands struct {
	Wait     []string `json:"wait"`
	Follow   []string `json:"follow"`
	Evidence []string `json:"evidence"`
	Cancel   []string `json:"cancel"`
	Logs     []string `json:"logs"`
}

// Handle is contract:ci.run-handle v1.
type Handle struct {
	Schema      string   `json:"schema"`
	RunID       string   `json:"run_id"`
	State       string   `json:"state"`
	PID         int      `json:"pid"`
	Coordinator string   `json:"coordinator"`
	StreamPath  string   `json:"stream_path"`
	Commands    Commands `json:"commands"`
}

// JobPlaceholder stands in the logs command when the job is not known at
// handle time (a bare run's jobs are planned after `queued`); the caller
// replaces it with a job id from `nself ci runs <run>`.
const JobPlaceholder = "<job>"

// NewHandle builds the handle of a queued run; coordinator is "local" or a URL;
// job names the job `logs` reads ("" = JobPlaceholder).
func NewHandle(runID string, pid int, coordinator, streamPath, job string) Handle {
	if job == "" {
		job = JobPlaceholder
	}
	if coordinator == "" {
		coordinator = Local
	}
	base := []string{"nself", "ci"}
	cmd := func(rest ...string) []string { return append(append([]string(nil), base...), rest...) }
	wait, follow := cmd("runs", "wait", runID), cmd("runs", "follow", runID, "--json")
	if coordinator != Local {
		wait = append(wait, "--coordinator", coordinator)
		follow = append(follow, "--coordinator", coordinator)
	}
	return Handle{Schema: Schema, RunID: runID, State: StateQueued, PID: pid, Coordinator: coordinator, StreamPath: streamPath,
		Commands: Commands{Wait: wait, Follow: follow, Evidence: cmd("evidence", runID, "--json"),
			Cancel: cmd("runs", "cancel", runID), Logs: cmd("logs", job, "--run", runID)}}
}
