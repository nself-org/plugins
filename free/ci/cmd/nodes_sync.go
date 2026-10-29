package main

// Purpose: `nself ci nodes sync (--from <file.yaml> | --cmd <argv...>) [--dry-run] [--json]` (P7-AUTO-04,
//          Epic AUTO D10): import an inventory/v1 document (a file, or the output of a discovery command
//          that runs through the exec funnel) over the node registry. New hosts become discovered
//          candidates; existing nodes get their operator-owned attributes (labels, pool.<id>, rank,
//          project skip rules, public_only) and nothing else.
// Inputs:  --from <file> or --cmd <argv...> (everything after --cmd is the command, so put flags first),
//          --dry-run, --json; env NSELF_CI_HOME.
// Outputs: a summary line block or one v1 JSON envelope (accepted, changed with field sources,
//          candidates, rejected, notes); exit 0, 1 (usage, E793, refusal), 2 (infrastructure).
// Constraints: V15 only; side-effect class: write; refused on a TEAM coordinator; never changes trust, host
//          keys, isolation or secrets eligibility, and never adds a project authorization; the repository
//          visibility reader for public_only is a seam (nodesSyncVisibility): none is wired yet, so
//          public_only nodes keep no project and the notes say why.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/nodes/inventory"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

const nodesSyncUsage = "usage: nself ci nodes sync (--from <file.yaml> | --cmd <argv...>) [--dry-run] [--json]"

// nodesSyncVisibility reads whether a project's repository is public; nil means every answer is unknown.
var nodesSyncVisibility inventory.Visibility

func nodesSyncFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("nodes sync", flag.ContinueOnError)
	fs.Bool("json", false, "print one v1 JSON envelope")
	fs.Bool("dry-run", false, "show what would change and write nothing")
	fs.String("from", "", "inventory/v1 file to import")
	fs.String("cmd", "", "command that prints an inventory/v1 document (every word after --cmd)")
	return fs
}

func init() {
	registerV15("nodes sync", subcommandEntry{
		V15Only: true,
		Flags:   nodesSyncFlags,
		Handler: func(args []string) int {
			return runNodesSync(args, lifecycle.ProductionEnv(currentProject), nodeAdminDeps.Load() != nil, funnelRunner, os.Stdout, os.Stderr)
		},
	})
}

// funnelRunner runs a discovery command through the exec funnel (report.Capture).
func funnelRunner(ctx context.Context, argv []string) (string, int, error) {
	return report.Capture(ctx, argv...)
}

type nodesSyncOpts struct {
	from           string
	cmd            []string
	dryRun, asJSON bool
}

// parseNodesSync reads argv: --cmd takes every following word.
func parseNodesSync(args []string) (nodesSyncOpts, error) {
	var o nodesSyncOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true":
			o.asJSON = true
		case a == "--dry-run" || a == "-dry-run" || a == "--dry-run=true" || a == "-dry-run=true":
			o.dryRun = true
		case a == "--from" || a == "-from":
			if i++; i >= len(args) {
				return o, placementUsage(nodesSyncUsage)
			}
			o.from = args[i]
		case strings.HasPrefix(a, "--from="):
			o.from = strings.TrimPrefix(a, "--from=")
		case a == "--cmd" || a == "-cmd":
			o.cmd = args[i+1:]
			i = len(args)
		default:
			return o, placementUsage(nodesSyncUsage)
		}
	}
	if (o.from == "") == (len(o.cmd) == 0) {
		return o, placementUsage(nodesSyncUsage)
	}
	return o, nil
}

// inventoryFail prints an inventory refusal (E793, exit 1); any other error goes to the placement printer.
func inventoryFail(out, errOut io.Writer, asJSON bool, err error) int {
	ie, ok := err.(*inventory.Error)
	if !ok {
		return placementFail(out, errOut, "ci nodes sync", asJSON, err)
	}
	if asJSON {
		_ = output.WriteError(out, "ci nodes sync", output.ErrorDetail{Code: ie.Code, Message: oneLine(ie), ExitCode: output.ExitUser}, nil)
		return output.ExitUser
	}
	_, _ = io.WriteString(errOut, oneLine(ie)+"\n")
	return output.ExitUser
}

// runNodesSync is the handler with its environment injected, for tests. team is true on a coordinator.
func runNodesSync(args []string, env lifecycle.Env, team bool, run inventory.Runner, out, errOut io.Writer) int {
	o, err := parseNodesSync(args)
	if err != nil {
		return placementFail(out, errOut, "ci nodes sync", hasJSONArg(args), err)
	}
	if team {
		return inventoryFail(out, errOut, o.asJSON, &inventory.Error{Code: inventory.CodeInventory, Reason: inventory.ReasonTeam,
			Msg: "sync is refused on a TEAM coordinator: its admin owns the nodes"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var doc inventory.Document
	if o.from != "" {
		doc, err = inventory.ReadFile(o.from, inventory.DefaultLimits)
	} else {
		doc, err = inventory.ReadCmd(ctx, run, o.cmd, inventory.DefaultLimits)
	}
	if err != nil {
		return inventoryFail(out, errOut, o.asJSON, err)
	}
	d, closeFn, err := env.Open(ctx, "")
	if err != nil {
		return placementFail(out, errOut, "ci nodes sync", o.asJSON, placementInfra("the CI store cannot be opened: "+oneLine(err)))
	}
	defer closeFn()
	sum, err := inventory.Sync(ctx, inventory.Env{Store: d.Store, Registry: d.Registry, Actor: d.Actor, Team: team, Visibility: nodesSyncVisibility},
		doc.Entries, inventory.Options{DryRun: o.dryRun})
	if err != nil {
		return inventoryFail(out, errOut, o.asJSON, err)
	}
	if o.asJSON {
		if output.WriteData(out, "ci nodes sync", sum, nil) != nil {
			return output.ExitInfra
		}
		return 0
	}
	renderSyncSummary(out, sum)
	return 0
}

func renderSyncSummary(out io.Writer, s inventory.Summary) {
	verb := "applied"
	if s.DryRun {
		verb = "would apply"
	}
	_, _ = fmt.Fprintf(out, "inventory: %d accepted, %d candidate(s), %d rejected; %s %d change(s)\n", len(s.Accepted), len(s.Candidates), len(s.Rejected), verb, len(s.Changed))
	for _, c := range s.Changed {
		_, _ = fmt.Fprintf(out, "  %s %s %s: %s -> %s (source %s)\n", c.Node, c.Field, c.Name, c.From, c.To, c.Source)
	}
	for _, c := range s.Candidates {
		_, _ = fmt.Fprintf(out, "  candidate %s (created %t): ineligible until `nself ci nodes add`\n", c.ID, c.Created)
	}
	for _, r := range s.Rejected {
		_, _ = fmt.Fprintf(out, "  rejected %s: %s\n", r.ID, r.Reason)
	}
	for _, n := range s.Notes {
		_, _ = fmt.Fprintf(out, "  note: %s\n", n)
	}
}
