package main

// Purpose: `nself ci nodes label <node|local> [key|key=value]... [--remove key...] [--json]` (P7-NODE-30,
// Epic NODE D19): coordinator-assigned node labels. Placement matches them against a job's
// requires.labels and the project policy sched.overrides.labels. With no label and no --remove the
// command lists the node's labels. `local` addresses the invoking machine's runner.
// Inputs:  the node id, labels and keys; env NSELF_CI_HOME; --coordinator <url> for a TEAM coordinator.
// Outputs: the node's labels as a line or one v1 JSON envelope (identity.labels, source assigned); exit 0,
//          1 (usage, E658, unknown node, refusal), 2 (infrastructure).
// Constraints: V15 only; side-effect class: write (list: read); every change writes one audit row per
//          label; this file also holds the helpers shared with nodes_projects.go (failure printing, the
//          project resolver, the coordinator runner) and registers the world label source.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const nodesLabelUsage = "usage: nself ci nodes label <node|local> [key|key=value]... [--remove key...] [--json] [--coordinator <url>]"

func nodesLabelFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("nodes label", flag.ContinueOnError)
	fs.Bool("json", false, "print one v1 JSON envelope")
	fs.String("remove", "", "remove these label keys (every word after --remove up to the next flag)")
	lifecycle.AddCoordinatorFlag(fs)
	return fs
}

func init() {
	registerV15("nodes label", subcommandEntry{
		V15Only: true,
		Flags:   nodesLabelFlags,
		Handler: func(args []string) int {
			if code, done := placementRemote("label", args, os.Stdout, os.Stderr); done {
				return code
			}
			return runNodesLabel(args, lifecycle.ProductionEnv(currentProject), false, os.Stdout, os.Stderr)
		},
	})
	reader := &labels.LazyReader{Path: func() string { return filepath.Join(lifecycle.CIHome(), "state.db") },
		Live: func() *store.Store {
			if d := nodeAdminDeps.Load(); d != nil {
				return d.store
			}
			return nil
		}}
	if err := labels.RegisterSource(reader); err != nil {
		panic("nself-ci: " + err.Error())
	}
	if err := clientapi.RegisterPlacementRoutes(func() clientapi.PlacementRunner {
		if nodeAdminDeps.Load() == nil {
			return nil
		}
		return runPlacementCommand
	}); err != nil {
		panic("nself-ci: " + err.Error())
	}
}

// parseNodesLabel splits argv into the node, the labels to set, the keys to remove and --json.
func parseNodesLabel(args []string) (node string, add, remove []string, asJSON bool, err error) {
	removing := false
	for _, a := range args {
		switch {
		case a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true":
			asJSON = true
		case a == "--remove" || a == "-remove":
			removing = true
		case strings.HasPrefix(a, "--remove=") || strings.HasPrefix(a, "-remove="):
			_, v, _ := strings.Cut(a, "=")
			remove = append(remove, v)
		case strings.HasPrefix(a, "-"):
			err = placementUsage("unknown flag " + a)
			return
		case node == "":
			node = strings.ToLower(a)
		case removing:
			remove = append(remove, a)
		default:
			add = append(add, a)
		}
	}
	if node == "" {
		err = placementUsage(nodesLabelUsage)
	} else if node != labels.LocalNode && remote.ValidateNodeID(node) != nil {
		err = placementUsage("the node id is not valid (letters, digits, dot, dash, underscore)")
	}
	return
}

// runNodesLabel is the handler with its environment injected, for tests. team is true on a coordinator.
func runNodesLabel(args []string, env lifecycle.Env, team bool, out, errOut io.Writer) int {
	node, add, remove, asJSON, err := parseNodesLabel(args)
	if err != nil {
		return placementFail(out, errOut, "ci nodes label", asJSON || hasJSONArg(args), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	d, closeFn, err := env.Open(ctx, "")
	if err != nil {
		return placementFail(out, errOut, "ci nodes label", asJSON, placementInfra("the CI store cannot be opened: "+oneLine(err)))
	}
	defer closeFn()
	e := placementEnv(d, team)
	var set []store.NodeLabel
	if len(add)+len(remove) == 0 {
		set, err = e.Labels(ctx, node)
	} else {
		set, err = e.Edit(ctx, node, add, remove)
	}
	if err != nil {
		return placementFail(out, errOut, "ci nodes label", asJSON, err)
	}
	if asJSON {
		items := make([]map[string]string, 0, len(set))
		for _, l := range set {
			items = append(items, map[string]string{"key": l.Key, "value": l.Value, "source": "assigned"})
		}
		data := map[string]any{"node": node, "identity": map[string]any{"labels": items}}
		if output.WriteData(out, "ci nodes label", data, nil) != nil {
			return output.ExitInfra
		}
		return 0
	}
	if len(set) == 0 {
		_, _ = fmt.Fprintf(out, "node %s: no labels\n", node)
		return 0
	}
	_, _ = fmt.Fprintf(out, "node %s: labels %s\n", node, strings.Join(labels.Strings(set), " "))
	return 0
}

// placementEnv is the label and project environment of one command.
func placementEnv(d *lifecycle.Deps, team bool) labels.Env {
	e := labels.Env{Store: d.Store, Registry: d.Registry, Actor: d.Actor, Team: team}
	if !team {
		e.ResolveDir = func(ctx context.Context, dir string) (string, bool) {
			root, err := filepath.Abs(dir)
			if err != nil {
				return "", false
			}
			id := policy.ProjectIDGit(ctx, policy.Runner(report.Capture), root)
			return id, id != ""
		}
	}
	return e
}

type placementError struct{ code, msg string }

func (e *placementError) Error() string { return e.msg }

func placementUsage(msg string) error { return &placementError{"E651", msg} }
func placementInfra(msg string) error { return &placementError{"E607", msg} }

func oneLine(err error) string { return strings.Join(strings.Fields(err.Error()), " ") }

// placementFail prints err (a JSON error envelope with --json) and returns its exit code.
func placementFail(out, errOut io.Writer, command string, asJSON bool, err error) int {
	code, exit := "E607", output.ExitInfra
	var pe *placementError
	var le *labels.Error
	var re *registry.Error
	var se *store.Error
	var lc *lifecycle.Error
	switch {
	case errors.As(err, &pe):
		code = pe.code
	case errors.As(err, &le):
		code = le.Code
	case errors.As(err, &re):
		code = re.Code
	case errors.As(err, &lc):
		code = lc.Code
	case errors.As(err, &se):
		code = se.Code
	}
	switch code {
	case "E650", "E651", "E657", "E658", "E664":
		exit = output.ExitUser
	}
	if asJSON {
		_ = output.WriteError(out, command, output.ErrorDetail{Code: code, Message: oneLine(err), ExitCode: exit}, nil)
		return exit
	}
	_, _ = io.WriteString(errOut, oneLine(err)+"\n")
	return exit
}
