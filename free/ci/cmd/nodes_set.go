package main

// Purpose: `nself ci nodes set <node> [--reserve-cpu N|P%] [--reserve-mem MB|Ng] [--interactive protect|ignore]
// [--on-battery pause|drain|allow] [--persistence persistent|intermittent]` (P7-NODE-13, Epic D10): the
// operator's contention policy for one node. Settings are stored in the registry (store.NodePutSettings)
// and mirrored into the node's capability snapshot, where the scheduler reads them.
// Inputs:  the node id and the flags above; env NSELF_CI_HOME, NSELF_CI_REPO.
// Outputs: the stored settings as a table line or one v1 JSON envelope; exit 0, 1 (usage, unknown node),
//          2 (infrastructure).
// Constraints: V15 only; flags not given keep the stored value (or the D10 default for a node never set);
//          at least one flag is required; side-effect class: write (registry only, no remote call).

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type nodesSetFlags struct {
	json                                      bool
	project, cpu, mem, interactive, onBattery string
	persistence                               string
}

func nodesSetFlagSet(f *nodesSetFlags, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes set", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&f.project, "project", "", "project id (default: the current repository)")
	fs.StringVar(&f.cpu, "reserve-cpu", "", "CPU kept for the operator: cores (2, 0.5) or a percentage of the node's cores (50%)")
	fs.StringVar(&f.mem, "reserve-mem", "", "memory kept for the operator: MB (2048) or GB (2g)")
	fs.StringVar(&f.interactive, "interactive", "", "protect (background work waits while the operator is active) or ignore")
	fs.StringVar(&f.onBattery, "on-battery", "", "pause (no new leases), drain (no new leases and drain the node) or allow")
	fs.StringVar(&f.persistence, "persistence", "", "persistent or intermittent (a laptop that sleeps)")
	lifecycle.AddCoordinatorFlag(fs)
	return fs
}

func init() {
	registerV15("nodes set", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { var f nodesSetFlags; return nodesSetFlagSet(&f, io.Discard) },
		Handler: func(args []string) int {
			return runNodesSet(args, lifecycle.ProductionEnv(currentProject), os.Stdout, os.Stderr)
		},
	})
}

// runNodesSet is the handler with its environment injected, for tests.
func runNodesSet(args []string, env lifecycle.Env, out, errOut io.Writer) int {
	var f nodesSetFlags
	fs := nodesSetFlagSet(&f, errOut)
	// The node id comes first; flags may follow it or precede it.
	pos, rest := nodesSetSplit(args)
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return nodesSetFail(out, errOut, f.json || nodesSetHasJSON(args), nodesSetUsage(err.Error()))
	}
	pos = append(pos, fs.Args()...)
	if len(pos) != 1 {
		return nodesSetFail(out, errOut, f.json, nodesSetUsage("usage: nself ci nodes set <node> [--reserve-cpu N|P%] [--reserve-mem MB|Ng] [--interactive protect|ignore] [--on-battery pause|drain|allow] [--persistence persistent|intermittent] [--json]"))
	}
	given := map[string]bool{}
	fs.Visit(func(fl *flag.Flag) { given[fl.Name] = true })
	if !given["reserve-cpu"] && !given["reserve-mem"] && !given["interactive"] && !given["on-battery"] && !given["persistence"] {
		return nodesSetFail(out, errOut, f.json, nodesSetUsage("nodes set needs at least one of --reserve-cpu, --reserve-mem, --interactive, --on-battery, --persistence"))
	}
	flagVals := reservation.Flags{CPU: f.cpu, Mem: f.mem, Interactive: f.interactive, OnBattery: f.onBattery, Persistence: f.persistence, Given: given}
	if err := flagVals.Check(); err != nil {
		return nodesSetFail(out, errOut, f.json, nodesSetUsage(err.Error()))
	}
	id := strings.ToLower(pos[0])
	if err := remote.ValidateNodeID(id); err != nil {
		return nodesSetFail(out, errOut, f.json, nodesSetUsage("the node id is not valid (letters, digits, dot, dash, underscore)"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	d, closeFn, err := env.Open(ctx, f.project)
	if err != nil {
		return nodesSetFail(out, errOut, f.json, nodesSetInfra("the CI store cannot be opened: "+nodesSetOneLine(err)))
	}
	defer closeFn()
	n, err := d.Registry.Get(ctx, id)
	if err != nil {
		return nodesSetFail(out, errOut, f.json, err)
	}
	cur, ok, err := d.Store.NodeSettingsGet(ctx, id)
	if err != nil {
		return nodesSetFail(out, errOut, f.json, nodesSetInfra(nodesSetOneLine(err)))
	}
	if !ok { // Version stays 0: the write must find no row
		own := n.Capability.Identity.Ownership.Value
		cur = reservation.Defaults(id, own != nil && *own == "operator", n.Capability.Availability.BatteryPct.Value != nil)
	}
	next, err := reservation.Merge(cur, flagVals, n.Capability.Resources.CPU.Value, n.Capability.Resources.MemMB.Value)
	if err != nil {
		return nodesSetFail(out, errOut, f.json, nodesSetUsage(err.Error()))
	}
	admin, err := store.NewAdminAuthority(d.Actor)
	if err != nil {
		return nodesSetFail(out, errOut, f.json, err)
	}
	saved, err := d.Store.NodePutSettings(ctx, next, admin)
	if err != nil {
		return nodesSetFail(out, errOut, f.json, err)
	}
	if f.json {
		if output.WriteData(out, "ci nodes set", map[string]any{"settings": saved}, nil) != nil {
			return output.ExitInfra
		}
		return 0
	}
	_, _ = fmt.Fprintf(out, "node %s: reserve cpu %g, mem %d MB; interactive %s; on battery %s; %s\n",
		saved.NodeID, saved.ReservedCPU, saved.ReservedMemMB, saved.Interactive, saved.OnBattery, saved.Persistence)
	return 0
}

// nodesSetSplit separates a leading node id from the flags after it.
func nodesSetSplit(args []string) (pos, rest []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[:1], args[1:]
	}
	return nil, args
}

func nodesSetHasJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true" {
			return true
		}
	}
	return false
}

type nodesSetError struct{ code, msg string }

func (e *nodesSetError) Error() string { return e.msg }

func nodesSetUsage(msg string) error { return &nodesSetError{"E651", msg} }
func nodesSetInfra(msg string) error { return &nodesSetError{"E607", msg} }

func nodesSetOneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

// nodesSetFail prints err (a JSON error envelope with --json) and returns its exit code.
func nodesSetFail(out, errOut io.Writer, asJSON bool, err error) int {
	code, exit := "E607", output.ExitInfra
	var se *nodesSetError
	var st *store.Error
	switch {
	case errors.As(err, &se):
		code = se.code
	case errors.As(err, &st):
		code = st.Code
	}
	if code == "E650" || code == "E651" {
		exit = output.ExitUser
	}
	if asJSON {
		_ = output.WriteError(out, "ci nodes set", output.ErrorDetail{Code: code, Message: err.Error(), ExitCode: exit}, nil)
		return exit
	}
	_, _ = io.WriteString(errOut, err.Error()+"\n")
	return exit
}
