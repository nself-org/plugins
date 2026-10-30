package main

// Purpose: `nself ci nodes projects add|remove|list <node> ...` (P7-NODE-30, Epic NODE D19): which projects
// may use a node. add and remove change lifecycle.authorized_projects through the registry (one audit row
// each); list reads it. A project is named by its repo id, or by a directory that resolves to it the way
// evidence does (`.` is the default). Also holds the client half of `--coordinator` for label and projects,
// and the runner behind the coordinator's /v1/nodes/{id}/labels and /projects routes.
// Inputs:  the node id, project arguments, --all (add: every project this PERSONAL store has seen;
//          remove: every authorized project), --json, --coordinator <url>; env NSELF_CI_HOME.
// Outputs: a table line or one v1 JSON envelope; exit 0, 1 (usage, E658, refusal), 2 (infrastructure).
// Constraints: V15 only; side-effect classes: add and remove write, list reads; removal lets a running
//          attempt finish and the lease gate refuses new leases (E657); `add --all` is refused on a TEAM
//          coordinator (E658 team_wildcard) and stores one explicit entry per project, never a wildcard.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

func nodesProjectsFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("nodes projects", flag.ContinueOnError)
	fs.Bool("json", false, "print one v1 JSON envelope")
	fs.Bool("all", false, "add: every project this store has seen (explicit entries); remove: every authorized project")
	lifecycle.AddCoordinatorFlag(fs)
	return fs
}

func init() {
	for _, sub := range []string{"add", "remove", "list"} {
		sub := sub
		registerV15("nodes projects "+sub, subcommandEntry{
			V15Only: true,
			Flags:   nodesProjectsFlags,
			Handler: func(args []string) int {
				if code, done := placementRemote("projects "+sub, args, os.Stdout, os.Stderr); done {
					return code
				}
				return runNodesProjects(sub, args, lifecycle.ProductionEnv(currentProject), false, os.Stdout, os.Stderr)
			},
		})
	}
}

// parseNodesProjects splits argv into the node, the project arguments and the flags.
func parseNodesProjects(sub string, args []string) (node string, projects []string, all, asJSON bool, err error) {
	for _, a := range args {
		switch {
		case a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true":
			asJSON = true
		case a == "--all" || a == "-all" || a == "--all=true":
			all = true
		case strings.HasPrefix(a, "-"):
			err = placementUsage("unknown flag " + a)
			return
		case node == "":
			node = strings.ToLower(a)
		default:
			projects = append(projects, a)
		}
	}
	usage := "usage: nself ci nodes projects " + map[string]string{
		"add":    "add <node> [<dir>|<repo-id>...] [--all] [--json]",
		"remove": "remove <node> <project...>|--all [--json]",
		"list":   "list <node> [--json]"}[sub] + " [--coordinator <url>]"
	switch {
	case node == "":
		err = placementUsage(usage)
	case remote.ValidateNodeID(node) != nil:
		err = placementUsage("the node id is not valid (letters, digits, dot, dash, underscore)")
	case sub == "list" && (len(projects) > 0 || all), all && len(projects) > 0, sub == "remove" && !all && len(projects) == 0:
		err = placementUsage(usage)
	}
	return
}

// runNodesProjects is the handler with its environment injected, for tests. team is true on a coordinator.
func runNodesProjects(sub string, args []string, env lifecycle.Env, team bool, out, errOut io.Writer) int {
	command := "ci nodes projects " + sub
	node, projects, all, asJSON, err := parseNodesProjects(sub, args)
	if err != nil {
		return placementFail(out, errOut, command, asJSON || hasJSONArg(args), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	d, closeFn, err := env.Open(ctx, "")
	if err != nil {
		return placementFail(out, errOut, command, asJSON, placementInfra("the CI store cannot be opened: "+oneLine(err)))
	}
	defer closeFn()
	e := placementEnv(d, team)
	var changed []string
	switch sub {
	case "add":
		changed, err = e.AddProjects(ctx, node, projects, all)
	case "remove":
		changed, err = e.RemoveProjects(ctx, node, projects, all)
	}
	if err != nil {
		return placementFail(out, errOut, command, asJSON, err)
	}
	now, err := e.Projects(ctx, node)
	if err != nil {
		return placementFail(out, errOut, command, asJSON, err)
	}
	if asJSON {
		data := map[string]any{"node": node, "authorized_projects": now}
		if sub != "list" {
			data["changed"] = append([]string{}, changed...)
		}
		if output.WriteData(out, command, data, nil) != nil {
			return output.ExitInfra
		}
		return 0
	}
	if sub != "list" {
		_, _ = fmt.Fprintf(out, "node %s: %s %d project(s)\n", node, map[string]string{"add": "authorized", "remove": "deauthorized"}[sub], len(changed))
	}
	if len(now) == 0 {
		_, _ = fmt.Fprintf(out, "node %s: no authorized projects\n", node)
		return 0
	}
	_, _ = fmt.Fprintf(out, "node %s: authorized projects %s\n", node, strings.Join(now, " "))
	return 0
}

// placementRemote is the client half of `--coordinator`: kind is "label" or "projects <add|remove|list>".
// done is false when argv has no --coordinator flag and the caller runs the local handler.
func placementRemote(kind string, args []string, out, errw io.Writer) (code int, done bool) {
	url, rest, found, err := takeCoordinatorFlag(args)
	if !found && err == nil {
		return 0, false
	}
	if coordOut != nil {
		out = coordOut
	}
	if coordErr != nil {
		errw = coordErr
	}
	asJSON := hasJSONArg(rest)
	var node string
	var req clientapi.PlacementRequest
	write := false
	if err == nil {
		if kind == "label" {
			var add, remove []string
			if node, add, remove, asJSON, err = parseNodesLabel(rest); err == nil {
				req = clientapi.PlacementRequest{JSON: asJSON, Labels: add, Remove: remove}
				write = len(add)+len(remove) > 0
			}
		} else {
			sub := strings.TrimPrefix(kind, "projects ")
			var projects []string
			var all bool
			if node, projects, all, asJSON, err = parseNodesProjects(sub, rest); err == nil {
				req = clientapi.PlacementRequest{JSON: asJSON, Action: sub, Projects: projects, All: all}
				write = sub != "list"
			}
		}
	}
	if err != nil {
		return placementFail(out, errw, "ci nodes "+kind, asJSON, err), true
	}
	coordinatorFlag = url
	defer func() { coordinatorFlag = "" }()
	cl, _, _, err := coordResolver.Client()
	if err != nil {
		return coordFail(out, errw, asJSON, err), true
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var res clientapi.NodeResult
	if kind == "label" {
		res, err = cl.NodeLabels(ctx, node, write, req)
	} else {
		res, err = cl.NodeProjects(ctx, node, write, req)
	}
	if err != nil {
		return coordFail(out, errw, asJSON, err), true
	}
	_, _ = io.WriteString(out, res.Stdout)
	_, _ = io.WriteString(errw, res.Stderr)
	return res.Exit, true
}

// runPlacementCommand is the clientapi.PlacementRunner: it runs the local handler of cmd.Verb as actor, on
// a coordinator (no directories, no `local`, no wildcard grants).
func runPlacementCommand(ctx context.Context, actor string, cmd clientapi.PlacementCommand) (clientapi.NodeResult, error) {
	st := nodeAdminDeps.Load()
	if st == nil {
		return clientapi.NodeResult{}, clientapi.Failf(clientapi.ReasonNotFound, "node administration is not available on this server")
	}
	var out, errw capWriter
	env := lifecycle.Env{Out: &out, Err: &errw,
		Open: func(_ context.Context, _ string) (*lifecycle.Deps, func(), error) {
			return lifecycle.NewDeps(st.store, st.home, "", actor), func() {}, nil
		}}
	args := []string{cmd.Node}
	var code int
	done := make(chan struct{})
	go func() {
		defer close(done)
		if cmd.Verb == clientapi.VerbLabel || cmd.Verb == clientapi.VerbLabelList {
			args = append(args, cmd.Labels...)
			if len(cmd.Remove) > 0 {
				args = append(args, append([]string{"--remove"}, cmd.Remove...)...)
			}
			if cmd.JSON {
				args = append(args, "--json")
			}
			code = runNodesLabel(args, env, true, &out, &errw)
			return
		}
		args = append(args, cmd.Projects...)
		if cmd.All {
			args = append(args, "--all")
		}
		if cmd.JSON {
			args = append(args, "--json")
		}
		code = runNodesProjects(strings.TrimPrefix(cmd.Verb, "projects "), args, env, true, &out, &errw)
	}()
	select {
	case <-done:
		if out.over || errw.over {
			return clientapi.NodeResult{Exit: max(code, 2), Stderr: "the answer is longer than the 1 MiB limit of the user API; read it on the coordinator host\n"}, nil
		}
		return clientapi.NodeResult{Exit: code, Stdout: out.b.String(), Stderr: errw.b.String()}, nil
	case <-ctx.Done():
		return clientapi.NodeResult{}, ctx.Err()
	}
}
