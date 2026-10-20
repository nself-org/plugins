package main

// Purpose: remove a retired project's cached deploy-host exclusions (D-1220).
// Inputs: an absolute project root and --json; the local CI store.
// Outputs: an audited removal and a confirmation or JSON envelope.
// Constraints: local administration only; other roots and the never-list stay.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

func nodesForgetProjectFlags(out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes forget-project", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Bool("json", false, "print one v1 JSON envelope")
	return fs
}

func init() {
	registerV15("nodes forget-project", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { return nodesForgetProjectFlags(io.Discard) },
		Handler: func(args []string) int {
			return runNodesForgetProject(args, lifecycle.ProductionEnv(currentProject), os.Stdout, os.Stderr)
		},
	})
}

func runNodesForgetProject(args []string, env lifecycle.Env, out, errOut io.Writer) int {
	fs := nodesForgetProjectFlags(errOut)
	var pos []string
	asJSON := nodesSetHasJSON(args)
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return nodesCommandFail(out, errOut, asJSON, "ci nodes forget-project", nodesSetUsage(err.Error()))
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(pos) != 1 || !filepath.IsAbs(pos[0]) {
		return nodesCommandFail(out, errOut, asJSON, "ci nodes forget-project", nodesSetUsage("usage: nself ci nodes forget-project <absolute-root> [--json]"))
	}
	ctx := context.Background()
	d, closeFn, err := env.Open(ctx, "")
	if err != nil {
		return nodesCommandFail(out, errOut, asJSON, "ci nodes forget-project", nodesSetInfra("the CI store cannot be opened"))
	}
	defer closeFn()
	root := filepath.Clean(pos[0])
	if err = deployguard.New(d.Store, d.Home, d.Actor).ForgetProject(ctx, root); err != nil {
		return nodesCommandFail(out, errOut, asJSON, "ci nodes forget-project", err)
	}
	if asJSON {
		if err = output.WriteData(out, "ci nodes forget-project", map[string]any{"forgotten": root}, nil); err != nil {
			return output.ExitInfra
		}
	} else {
		_, _ = fmt.Fprintf(out, "forgot project %s\n", root)
	}
	return 0
}
