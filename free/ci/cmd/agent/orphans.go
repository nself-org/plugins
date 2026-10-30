package main

// Purpose: a job must not outlive the agent that started it (D-1002). Every
// agent process names its jobs' coordinator by an incarnation id (the
// coordinator plus the agent's pid and start time); the executor records each
// job's process group under that id in a persistent home, and the next run's
// startup sweep kills the groups whose incarnation is no longer live.
// Inputs:  the coordinator id from the spec; the state of the process table.
// Outputs: agentIncarnation ids and the agentStale checker the executor uses.
// Constraints: ids hold only [A-Za-z0-9.-] after the "ag-" prefix and use "-"
// as the field separator, so they pass the executor's safe-id check; ids that
// do not parse (such as "local") are never reported stale.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// agentIncarnation is the coordinator id this process gives its jobs.
func agentIncarnation(coordinator string) string {
	self := slots.Self(coordinator, "")
	start := self.Start
	if start == "" {
		start = "0"
	}
	sum := sha256.Sum256([]byte(coordinator))
	return "ag-" + hex.EncodeToString(sum[:5]) + "-" + strconv.Itoa(self.PID) + "-" + start
}

// agentStale reports an incarnation id whose agent process is gone (or whose
// pid now belongs to another process).
type agentStale struct{}

func (agentStale) Stale(_ context.Context, id string) (bool, error) {
	parts := strings.Split(id, "-")
	if len(parts) != 4 || parts[0] != "ag" {
		return false, nil
	}
	pid, err := strconv.Atoi(parts[2])
	if err != nil || pid <= 0 {
		return false, nil
	}
	start := parts[3]
	if start == "0" {
		start = ""
	}
	return !slots.HolderLive(slots.Owner{PID: pid, Start: start}), nil
}

// execHome is the executor home, kept under the work base so job process
// records survive the agent and the next start can sweep them.
func execHome(base string) (string, error) {
	home := filepath.Join(base, "exec-home")
	return home, os.MkdirAll(home, 0o700)
}

// sweepOrphanJobs kills the job process groups an earlier incarnation of this agent left behind (a SIGKILLed
// agent cannot stop its jobs). runPull calls it before the first session: until then the orphan would run to
// its end, and finish a side effect the coordinator has already handed to another node (P7-NODE-11). A sweep
// that cannot run is reported and does not stop the agent.
func sweepOrphanJobs(ctx context.Context, coordinator string, logf func(string, ...any)) {
	base, err := workBase()
	if err != nil {
		logf("orphan sweep: %v", err)
		return
	}
	home, err := execHome(base)
	if err != nil {
		logf("orphan sweep: %v", err)
		return
	}
	ex := &ciexec.Executor{Stale: agentStale{}}
	if err = ex.SweepStaleProcesses(ctx, home, agentIncarnation(coordinator)); err != nil {
		logf("orphan sweep: %v", err)
	}
}
