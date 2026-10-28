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
	"os"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
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
			Handler:    func(args []string) int { return lifecycle.Run(name, args, lifecycle.ProductionEnv(currentProject)) },
		})
	}
}
