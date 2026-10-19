package main

// Purpose: `nodes add --allow-deploy-host` and the deploy-host check at add, pair and test (P7-NODE-14,
// Epic D9), hung on the node lifecycle's extension points so cmd/main.go and nodes_lifecycle.go stay
// untouched.
// Inputs:  the lifecycle hook event (destination, draft capability, flags); $NSELF_CI_HOME.
// Outputs: the draft's deploy_host fact at add; the stored node's fact at test; E663 when an override is
//          not confirmed or a host cannot be marked; warnings and notices on stderr.
// Constraints: the override needs an interactive terminal and the node id typed back; one audit row per
//          confirmed override; the current project root is recorded so every project's inventory joins
//          the node-global set.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

const allowDeployHostFlag = "allow-deploy-host"

// newGuard builds the coordinator-side guard; tests replace it with stub inventory and DNS seams.
var newGuard = deployguard.New

func init() {
	lifecycle.RegisterFlags("add", func(fs *flag.FlagSet) {
		fs.Bool(allowDeployHostFlag, false, "allow a node whose host is a deploy host (confirmed on a terminal, audited)")
	})
	for _, p := range []lifecycle.Point{lifecycle.PointAdd, lifecycle.PointPair, lifecycle.PointTest} {
		point := p
		lifecycle.RegisterAddHook(point, func(ctx context.Context, e lifecycle.HookEvent) error { return deployHostHook(ctx, point, e) })
	}
}

func adminActor() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return "cli:" + u.Username
	}
	return "cli:admin"
}

// projectRoot is the nearest directory above the working directory (or $NSELF_CI_REPO) holding .nself or .git.
func projectRoot() string {
	dir := os.Getenv("NSELF_CI_REPO")
	if dir == "" {
		dir, _ = os.Getwd()
	}
	for d := dir; d != "" && d != filepath.Dir(d); d = filepath.Dir(d) {
		for _, m := range []string{".nself", ".git"} {
			if _, err := os.Stat(filepath.Join(d, m)); err == nil {
				return d
			}
		}
	}
	return dir
}

// isTTY reports a terminal: a character device that is not /dev/null (also a character device).
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(fi, null)
}

func flagBool(fs *flag.FlagSet, name string) bool {
	if fs == nil {
		return false
	}
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	g, ok := f.Value.(flag.Getter)
	if !ok {
		return false
	}
	b, _ := g.Get().(bool)
	return b
}

func deployHostHook(ctx context.Context, point lifecycle.Point, e lifecycle.HookEvent) error {
	home := lifecycle.CIHome()
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		return fmt.Errorf("deploy-host check: %w", err)
	}
	defer func() { _ = st.Close() }()
	g := newGuard(st, home, adminActor())
	if err = g.Seen(ctx, projectRoot()); err != nil {
		return err
	}
	if point == lifecycle.PointTest {
		return reportWarnings(g.Refresh(ctx))
	}
	q := deployguard.AddRequest{NodeID: e.NodeID, Dest: e.Dest, Capability: e.Capability, Actor: adminActor(),
		Allow: flagBool(e.Flags, allowDeployHostFlag), Confirm: deployguard.TerminalConfirm(os.Stdin, os.Stderr, isTTY(os.Stdin))}
	r, warns, err := g.CheckAdd(ctx, q)
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if err != nil {
		return err
	}
	if r.Matched && e.Capability == nil {
		if _, eerr := g.EvaluateNode(ctx, e.NodeID); eerr != nil {
			return fmt.Errorf("node %s is a deploy host and cannot be paired: %w", e.NodeID, eerr)
		}
	}
	if r.Matched && !q.Allow {
		fmt.Fprintf(os.Stderr, "notice: %s is a deploy host (%s); it is registered but CI-ineligible\n", e.NodeID, r.Source)
	}
	return nil
}

func reportWarnings(err error) error {
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	return nil
}
