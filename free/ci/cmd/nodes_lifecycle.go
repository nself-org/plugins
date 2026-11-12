package main

// Purpose: the nine node lifecycle subcommands of `nself ci nodes` (P7-NODE-05, Epic D17), registered in
// the V15 subcommand table with multi-token keys. The implementation lives in internal/nodes/lifecycle;
// this file registers the keys and resolves the current project, nothing else, so cmd/main.go is
// untouched.
// Inputs:  argv after the key; env NSELF_CI_HOME, NSELF_CI_REPO.
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (usage, unknown node), 2 (infrastructure) or 3
//          (host key refused, node revoked).
// Constraints: V15 only, so v1.4 argv is unchanged (`nodes add` without NSELF_V15 is the default gate
//          path as before). Side-effect classes: list, show read; add, test, drain, enable, rotate
//          remote; revoke, remove destructive.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// nodesLifecycleSubs are the keys after "nodes", with their side-effect class.
var nodesLifecycleSubs = []struct {
	name  string
	class SideEffect
}{
	{"add", SideEffectRemote}, {"list", SideEffectRead}, {"show", SideEffectRead}, {"test", SideEffectRemote},
	{"drain", SideEffectRemote}, {"enable", SideEffectRemote}, {"revoke", SideEffectDestructive},
	{"remove", SideEffectDestructive}, {"rotate", SideEffectRemote},
}

// currentProject is the project id of the repository the command runs in.
func currentProject(ctx context.Context) string {
	root := os.Getenv("NSELF_CI_REPO")
	if root == "" {
		root, _ = os.Getwd()
	}
	if root == "" {
		return ""
	}
	return policy.ProjectIDGit(ctx, policy.Runner(report.Capture), root)
}

func init() {
	for _, s := range nodesLifecycleSubs {
		name := s.name
		registerV15("nodes "+name, subcommandEntry{
			V15Only:    true,
			SideEffect: s.class,
			Flags:      func() *flag.FlagSet { return lifecycle.FlagSet(name) },
			Handler: func(args []string) int {
				rc := lifecycle.Run(name, args, lifecycle.ProductionEnv(currentProject))
				if name == "revoke" || name == "remove" {
					rc = revokeEvidence(name, args, rc)
				}
				return rc
			},
		})
	}
}

// revokeEvidence revokes the node's signed evidence after `nodes revoke|remove` (P7-WIRE-11): its keys,
// the runs they signed and the releases promoted on them. It runs when the node is revoked (exit 0, or 3
// for a revoke that committed with incomplete cleanup) and is idempotent, so a repeated revoke retries it.
// A failure is reported and turns exit 0 into 2: the node is revoked but its evidence is not yet.
func revokeEvidence(sub string, args []string, rc int) int {
	if rc != 0 && rc != 3 {
		return rc
	}
	id := nodeArg(sub, args)
	if id == "" {
		return rc
	}
	home := lifecycle.CIHome()
	err := func() error {
		st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()
		_, err = cirun.RevokeNode(context.Background(), st, &audit.Log{St: st, Scrub: cirun.Redactions}, id, "cli:nodes-"+sub)
		return err
	}()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "nself ci nodes %s: node %s is revoked but its evidence revocation failed: %v\n", sub, id, err)
		if rc == 0 {
			return 2
		}
	}
	return rc
}

// nodeArg is the node id of a revoke or remove argv: its one positional argument, lower-cased as the
// command does.
func nodeArg(sub string, args []string) string {
	fs := lifecycle.FlagSet(sub)
	fs.SetOutput(io.Discard)
	var pos []string
	for rest := args; ; {
		if fs.Parse(rest) != nil {
			return ""
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(pos) != 1 {
		return ""
	}
	return strings.ToLower(pos[0])
}
