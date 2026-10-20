package main

// Purpose: `nself ci --detach` (P7-CI-63, Epic D30): start the run in the
// background and print contract:ci.run-handle v1 once the run is queued, so an
// agent or script can start → poll (`ci runs wait`) or stream (`ci runs
// follow`) → read the result without holding a connection for the whole run.
// Inputs:  the bare argv (also with `-- <cmd…>` and `--coordinator`).
// Outputs: one v1 envelope (--json) or three lines of text on stdout; exit 0,
// or 2 with E648 (reason spawn, no_queued_event, child_exit).
// Constraints: V15 only (the flag lives on the V15 bare flag set); the child is
// the ordinary engine re-executed with --json --stream, so this file adds no
// run loop, store writer or event format; the child never inherits
// NSELF_MCP_TOKEN; `runs prune` also removes the stream files of the runs it
// pruned. The flag hook runs in registration order, so flags registered by
// later files are validated by the child, whose error comes back as E648.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/detach"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// detachFlag is the --detach value of the current invocation; the hook reads
// and clears it (a process runs one invocation; tests parse several).
var detachFlag bool

// Seams for tests; production uses the process's own argv, binary and stdout.
var (
	detachArgs = func() []string { return os.Args[1:] }
	detachExe  = os.Executable
	detachExit = os.Exit
	detachOut  io.Writer
	detachTune func(*detach.Params)
)

func init() {
	prune := runsSubs["prune"] // registered by runs_history.go, which initialises first
	if prune == nil {
		panic("nself-ci: runs prune must register before cmd/v15_detach.go")
	}
	runsSubs["prune"] = func(args []string) int { return pruneStreams(prune, args) }
	bareFlags.BoolVar(&detachFlag, "detach", false, "start the run in the background and print a run handle once it is queued (see: nself ci runs wait, nself ci runs follow)")
	RegisterOptionsHook(detachOptions)
}

// detachOptions starts the detached run and exits with its handle.
func detachOptions(o *RunOptions) error {
	on := detachFlag
	detachFlag = false
	if !on {
		return nil
	}
	detachExit(runDetach(o))
	return nil // detachExit does not return in production
}

func runDetach(o *RunOptions) int {
	out := detachOut
	if out == nil {
		out = os.Stdout
	}
	exe, err := detachExe()
	if err != nil {
		return detachFail(out, o.JSON, &detach.Error{Reason: detach.ReasonSpawn, Message: err.Error(), Err: err})
	}
	p := detach.Params{Exe: exe, Args: detachArgs(), Home: o.Home, Env: os.Environ()}
	if detach.CoordinatorOf(p.Args, os.Getenv) == detach.Local && !coordResolver.Active() { // P7-NODE-19: a live serve or a user-config coordinator owns the runs; never open its store
		v := &localVerifier{since: time.Now()}
		defer v.close()
		p.Verify = v.exists // stderr shares the stream file: only a run in the store is believed
	}
	if detachTune != nil {
		detachTune(&p)
	}
	sweepOrphanStreams() // stream files of runs the retention prune already deleted
	h, err := detach.Start(p)
	if err != nil {
		return detachFail(out, o.JSON, err)
	}
	if o.JSON {
		if err = output.WriteData(out, "ci", h, nil); err != nil {
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "run %s %s (pid %d)\n  wait:   %s\n  follow: %s\n  cancel: %s\n", h.RunID, h.State, h.PID,
		strings.Join(h.Commands.Wait, " "), strings.Join(h.Commands.Follow, " "), strings.Join(h.Commands.Cancel, " "))
	return 0
}

// detachFail prints a failed detach in the mode the caller asked for.
func detachFail(out io.Writer, asJSON bool, err error) int {
	if asJSON {
		msg := err.Error()
		var de *detach.Error
		if errors.As(err, &de) {
			msg = de.Detail()
		}
		d := output.ErrorDetail{Code: detach.CodeID, Message: msg, ExitCode: 2, Class: "infra",
			Remediation: "Run the same command without --detach to see the error"}
		if output.WriteError(out, "ci", d, nil) == nil {
			return 2
		}
	}
	fmt.Fprintln(os.Stderr, err)
	return 2
}

// pruneStreams runs `runs prune` and then removes the detached stream files of
// the runs it deleted, plus files of runs that are gone for good.
func pruneStreams(prune handler, args []string) int {
	if !hasDryRun(args) { // P7-NODE-19: knownRunIDs opens the store, which a live serve owns
		if code, ok := refuseWhileServe("`runs prune`"); ok {
			return code
		}
	}
	before := knownRunIDs()
	code := prune(args)
	if code != 0 || hasDryRun(args) || before == nil {
		return code
	}
	after := knownRunIDs()
	home := detach.Home("")
	var gone []string
	for id := range before {
		if !after[id] {
			gone = append(gone, id)
		}
	}
	_, _ = detach.Remove(home, gone)
	_, _ = detach.Sweep(home, func(id string) bool { return after[id] }, time.Time{}, false)
	return code
}

// sweepOrphanStreams removes detached stream files whose run row is gone, so
// terminal-run retention (which prunes rows at every run start) frees them too.
func sweepOrphanStreams() {
	if known := knownRunIDs(); known != nil {
		_, _ = detach.Sweep(detach.Home(""), func(id string) bool { return known[id] }, time.Time{}, false)
	}
}

func hasDryRun(args []string) bool {
	for _, a := range args {
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || name != "dry-run" {
			continue
		}
		if b, err := strconv.ParseBool(val); !hasVal || (err == nil && b) {
			return true
		}
	}
	return false
}

// knownRunIDs lists the run ids in the store; nil when there is no history.
func knownRunIDs() map[string]bool {
	if _, err := os.Stat(filepath.Join(detach.Home(""), "state.db")); err != nil {
		return nil
	}
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return nil
	}
	defer func() { _ = st.Close() }()
	ages, err := st.PipelineAges(context.Background())
	if err != nil {
		return nil
	}
	ids := make(map[string]bool, len(ages))
	for _, a := range ages {
		ids[a.ID] = true
	}
	return ids
}

// localVerifier answers "is this a run this detach started": a run of the local
// store created after the child was spawned (a stderr line naming an older run
// is not believed). It opens the store once, after the child has created it.
type localVerifier struct {
	st    *store.Store
	since time.Time
}

func (v *localVerifier) exists(runID string) bool {
	if v.st == nil {
		if _, err := os.Stat(filepath.Join(detach.Home(""), "state.db")); err != nil {
			return false
		}
		st, err := cirun.OpenStore(runsStorePath())
		if err != nil {
			return false
		}
		v.st = st
	}
	ages, err := v.st.PipelineAges(context.Background())
	if err != nil {
		return false
	}
	for _, a := range ages {
		if a.ID == runID {
			return a.CreatedAt >= v.since.UnixNano()
		}
	}
	return false
}

func (v *localVerifier) close() {
	if v.st != nil {
		_ = v.st.Close()
	}
}
