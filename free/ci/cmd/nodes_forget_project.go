package main

// Purpose: `nself ci nodes forget-project <root> [--json]` (D-1220): drop a project root from the
// node-global deploy-host exclusion set, with its cached entries and refresh state (Guard.ForgetProject,
// one audit row). It is how an operator clears a project that was deleted or no longer deploys; until then
// a root that can no longer be read keeps `nodes add` on the unverified rule.
// Inputs:  the absolute project root as the exclusion set knows it; env NSELF_CI_HOME.
// Outputs: one line naming the root, or one v1 JSON envelope; exit 0, 1 (usage, root not known) or 2
//          (the CI store cannot be opened or written).
// Constraints: V15 only; side-effect class: write (local cache only, no remote call, no inventory read);
//          a root the cache does not hold is refused so a typo cannot pass for a success.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

const forgetProjectUsage = "usage: nself ci nodes forget-project <absolute project root> [--json]"

func forgetProjectFlagSet(asJSON *bool, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes forget-project", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.BoolVar(asJSON, "json", false, "print one v1 JSON envelope")
	return fs
}

func init() {
	registerV15("nodes forget-project", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { var j bool; return forgetProjectFlagSet(&j, io.Discard) },
		Handler: func(args []string) int { return runNodesForgetProject(args, os.Stdout, os.Stderr) },
	})
}

// runNodesForgetProject is the handler with its writers injected, for tests.
func runNodesForgetProject(args []string, out, errOut io.Writer) int {
	var asJSON bool
	fs := forgetProjectFlagSet(&asJSON, errOut)
	// The root comes first or last; flags may sit on either side of it.
	var pos []string
	rest := args
	for len(rest) > 0 {
		if strings.HasPrefix(rest[0], "-") {
			if err := fs.Parse(rest); err != nil {
				if errors.Is(err, flag.ErrHelp) {
					return 0
				}
				return forgetProjectFail(out, errOut, asJSON, "E651", err.Error(), output.ExitUser)
			}
			rest = fs.Args()
			continue
		}
		pos, rest = append(pos, rest[0]), rest[1:]
	}
	if len(pos) != 1 {
		return forgetProjectFail(out, errOut, asJSON, "E651", forgetProjectUsage, output.ExitUser)
	}
	root := pos[0]
	if !filepath.IsAbs(root) {
		return forgetProjectFail(out, errOut, asJSON, "E651", "the project root must be an absolute path, as the exclusion set stores it", output.ExitUser)
	}
	root = filepath.Clean(root)
	home := lifecycle.CIHome()
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		return forgetProjectFail(out, errOut, asJSON, "E607", "the CI store cannot be opened: "+nodesSetOneLine(err), output.ExitInfra)
	}
	defer func() { _ = st.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	known, err := st.ExclusionProjects(ctx)
	if err != nil {
		return forgetProjectFail(out, errOut, asJSON, "E607", nodesSetOneLine(err), output.ExitInfra)
	}
	if !slices.Contains(known, root) {
		return forgetProjectFail(out, errOut, asJSON, "E651", fmt.Sprintf("%s is not a known project root (known: %s)", root, strings.Join(known, ", ")), output.ExitUser)
	}
	if err = newGuard(st, home, adminActor()).ForgetProject(ctx, root); err != nil {
		return forgetProjectFail(out, errOut, asJSON, "E607", nodesSetOneLine(err), output.ExitInfra)
	}
	if asJSON {
		if output.WriteData(out, "ci nodes forget-project", map[string]any{"root": root, "forgotten": true}, nil) != nil {
			return output.ExitInfra
		}
		return 0
	}
	_, _ = fmt.Fprintf(out, "forgot project root %s (its cached deploy hosts are no longer excluded)\n", root)
	return 0
}

// forgetProjectFail prints the failure (a JSON error envelope with --json) and returns its exit code.
func forgetProjectFail(out, errOut io.Writer, asJSON bool, code, msg string, exit int) int {
	if asJSON {
		_ = output.WriteError(out, "ci nodes forget-project", output.ErrorDetail{Code: code, Message: msg, ExitCode: exit}, nil)
		return exit
	}
	_, _ = io.WriteString(errOut, msg+"\n")
	return exit
}
