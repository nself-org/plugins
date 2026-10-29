package main

// Purpose: `nself ci nodes status [--local] [--json]` (P7-AUTO-19): the fleet at a glance. --local prints
//          this host's demand document (contract:ci.demand v1). Without it every registered node is asked,
//          8 at a time and under a hard 8 s cap each, for its demand document and readiness block through
//          the probe package's fixed read-only commands; the answers are merged with the registry (state,
//          agent version), the pool of each node and the local store's last results.
// Inputs:  --local, --json; env NSELF_CI_HOME (the store, the pin file known_hosts, the demand file).
// Outputs: a table or one v1 JSON envelope; exit 0 (also when nodes are unreachable: they are rows),
//          1 (usage), 2 (infrastructure).
// Constraints: V15 only; side-effect class read. No node value enters a remote command; a dead or slow node
//          is reported unreachable with its reason and never delays the others. `ci prepare` is superseded
//          (NODE-03 ships the revision as a bundle per lease): no remote checkout is created or refreshed.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/fleet"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const nodesStatusCommand = "ci nodes status"

type nodesStatusOpts struct {
	json, local bool
}

func newNodesStatusFlags(errOut io.Writer) (*flag.FlagSet, *nodesStatusOpts) {
	o := &nodesStatusOpts{}
	fs := flag.NewFlagSet("nodes status", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.BoolVar(&o.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&o.local, "local", false, "print this host's demand document only")
	return fs, o
}

// nodesStatusDeps are the outside readers of the command.
type nodesStatusDeps struct {
	Home string
	// Open opens the existing store (nil store, nil error when there is none yet).
	Open func(path string) (*store.Store, error)
	// Reader builds the per-node status reader.
	Reader  func(ctx context.Context, home string) fleet.Reader
	Options fleet.Options
}

func init() {
	registerV15("nodes status", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { fs, _ := newNodesStatusFlags(io.Discard); return fs },
		Handler: func(args []string) int {
			d := nodesStatusDeps{Home: lifecycle.CIHome(), Open: openStatusStore, Reader: sshStatusReader}
			return runNodesStatus(args, d, os.Stdout, os.Stderr)
		},
	})
}

// runNodesStatus is the handler with its environment injected.
func runNodesStatus(args []string, d nodesStatusDeps, out, errOut io.Writer) int {
	fs, o := newNodesStatusFlags(io.Discard)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(errOut)
			fs.Usage()
			return 0
		}
		return placementFail(out, errOut, nodesStatusCommand, o.json || hasJSONArg(args), placementUsage("usage: nself ci nodes status [--local] [--json]"))
	}
	if fs.NArg() != 0 {
		return placementFail(out, errOut, nodesStatusCommand, o.json, placementUsage("usage: nself ci nodes status [--local] [--json]"))
	}
	if o.local {
		return nodesStatusLocal(o.json, d, out, errOut)
	}
	ctx := context.Background()
	st, err := d.Open(filepath.Join(d.Home, "state.db"))
	if err != nil {
		return placementFail(out, errOut, nodesStatusCommand, o.json, placementInfra("open the state database: "+oneLine(err)))
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	rep, err := fleet.Gather(ctx, st, d.Reader(ctx, d.Home), d.Options)
	if err != nil {
		return placementFail(out, errOut, nodesStatusCommand, o.json, placementInfra("read the registry: "+oneLine(err)))
	}
	if o.json {
		if err := output.WriteData(out, nodesStatusCommand, rep, nil); err != nil {
			return placementFail(out, errOut, nodesStatusCommand, false, placementInfra(oneLine(err)))
		}
		return 0
	}
	if err := fleet.Render(out, rep); err != nil {
		return output.ExitInfra
	}
	return 0
}

// localDemand is the --local document: the demand file plus where it came from.
type localDemand struct {
	demand.Document
	Source string `json:"source"`
}

// nodesStatusLocal prints this host's demand document.
func nodesStatusLocal(asJSON bool, d nodesStatusDeps, out, errOut io.Writer) int {
	snap, err := demand.Read(demand.ReadConfig{Dir: d.Home})
	if err != nil {
		return placementFail(out, errOut, nodesStatusCommand, asJSON, placementInfra(oneLine(err)))
	}
	doc := localDemand{Document: snap.Doc, Source: fleet.SourceLive}
	if snap.Doc.TS == "" {
		doc.Source = "none" // no watcher publishes on this host
	}
	if doc.ReadyWhy == nil {
		doc.ReadyWhy = []string{}
	}
	if asJSON {
		if err := output.WriteData(out, nodesStatusCommand, doc, nil); err != nil {
			return placementFail(out, errOut, nodesStatusCommand, false, placementInfra(oneLine(err)))
		}
		return 0
	}
	if doc.Source == "none" {
		fmt.Fprintln(out, "watcher: none (this host publishes no demand document)")
		return 0
	}
	ready := "ready"
	if !doc.Ready {
		ready = "not ready"
	}
	fmt.Fprintf(out, "host: %s\nready: %s\nwatcher: alive, last pass %s (%s ago)\nqueued: %d  held: %d  running: %d\n",
		doc.Host, ready, doc.Watcher.LastPass, snap.Age.Round(time.Second), doc.Queued, doc.Held, doc.Running)
	for _, why := range doc.ReadyWhy {
		fmt.Fprintf(out, "  why: %s\n", why)
	}
	return 0
}

// openStatusStore opens the state database only when it exists: a status read never creates one.
func openStatusStore(path string) (*store.Store, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return store.Open(path, store.Options{})
}

// sshStatusReader reads each node over the SDK remote funnel with the pinned D4 option set.
func sshStatusReader(ctx context.Context, home string) fleet.Reader {
	vctx, cancel := context.WithTimeout(ctx, fleet.NodeCap)
	defer cancel()
	v, verr := remote.SSHVersion(vctx)
	pinned := filepath.Join(home, "known_hosts")
	return func(ctx context.Context, n registry.Node) (probe.StatusDocs, error) {
		if verr != nil {
			return probe.StatusDocs{}, fmt.Errorf("local ssh: %w", verr)
		}
		run, err := probe.SSHStatusRunner(n, pinned, "", v)
		if err != nil {
			return probe.StatusDocs{}, err
		}
		return probe.ReadStatus(ctx, run)
	}
}
