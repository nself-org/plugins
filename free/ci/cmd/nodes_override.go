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
	"sync"

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
	for _, p := range []lifecycle.Point{lifecycle.PointPreAdd, lifecycle.PointAdd, lifecycle.PointPair, lifecycle.PointTest} {
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

// hasNself reports a project that deploys: only such roots join the node-global set, so a stray git
// checkout never becomes a root that can fail to read.
func hasNself(root string) bool {
	fi, err := os.Stat(filepath.Join(root, ".nself"))
	return err == nil && fi.IsDir()
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

// preConfirmed remembers the nodes whose --allow-deploy-host override was confirmed and audited before
// contact, so the add hook that follows the probe marks the draft without asking again.
var preConfirmed sync.Map

func deployHostHook(ctx context.Context, point lifecycle.Point, e lifecycle.HookEvent) error {
	home := lifecycle.CIHome()
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		return fmt.Errorf("deploy-host check: %w", err)
	}
	defer func() { _ = st.Close() }()
	g := newGuard(st, home, adminActor())
	if root := projectRoot(); hasNself(root) {
		if err = g.Seen(ctx, root); err != nil {
			return err
		}
	}
	if point == lifecycle.PointTest {
		return reportWarnings(g.Refresh(ctx))
	}
	q := deployguard.AddRequest{NodeID: e.NodeID, Dest: e.Dest, Capability: e.Capability, Actor: adminActor(),
		Allow: flagBool(e.Flags, allowDeployHostFlag), Confirm: deployguard.TerminalConfirm(os.Stdin, os.Stderr, isTerminal(os.Stdin))}
	check := g.CheckAdd
	override := false
	switch point {
	case lifecycle.PointPreAdd:
		preConfirmed.Delete(e.NodeID)
		check = g.CheckPreAdd
	case lifecycle.PointAdd:
		// Confirmed and audited before contact: match again with the probed identity, without a second prompt.
		if _, ok := preConfirmed.LoadAndDelete(e.NodeID); ok && q.Allow {
			q.Allow, override = false, true
		}
	}
	r, warns, err := check(ctx, q)
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if err != nil {
		return err
	}
	if point == lifecycle.PointPreAdd {
		if r.Matched {
			preConfirmed.Store(e.NodeID, true)
		}
		return nil
	}
	if override && r.Matched && e.Capability != nil {
		e.Capability.DeployHost.Override = true
	}
	if r.Matched && e.Capability == nil {
		// No draft to mark: the stored node must carry the match, or the pairing is refused.
		if again, eerr := g.EvaluateNode(ctx, e.NodeID); eerr != nil || !again.Matched {
			if eerr != nil {
				return fmt.Errorf("node %s is a deploy host and cannot be paired: %w", e.NodeID, eerr)
			}
			return fmt.Errorf("node %s is a deploy host and cannot be paired: the stored node does not carry the match", e.NodeID)
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
