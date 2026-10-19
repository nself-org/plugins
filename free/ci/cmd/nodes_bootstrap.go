package main

// Purpose: `nodes add --system` (P7-NODE-06, Epic D8): the flags and the agent installer that turn a node
// into a separated system node (agent user `nself-ci`, one job user per slot, one sudoers rule per slot
// user for the job shim only, the agent under /opt/nself-ci-agent). The work is in
// internal/nodes/bootstrap; this file registers the flags, the "system" installer and a guard hook.
// Inputs:  `--system` (selects the installer), `--slots N` (job users, default 2), `--agent-binary`.
// Outputs: a node that `nodes add` then records as uid-separated (system.users): jobs never run under
//          the agent's UID, the node key is readable by the agent user alone.
// Constraints: the ssh user must be root or have passwordless sudo (E669 no_root); `--system` without
//          the installer having run (no --agent-binary, or another --installer) fails the add before the
//          node is registered; a macOS node without a registered family gets the brew commands printed;
//          cmd/main.go and internal/nodes/lifecycle are untouched, the flag steers the installer flag.

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

const (
	systemInstaller = "system"
	defaultSlots    = 2
)

// systemFlag is the bool flag `--system`: setting it selects the system installer.
type systemFlag struct {
	fs *flag.FlagSet
	on bool
}

func (s *systemFlag) String() string   { return strconv.FormatBool(s != nil && s.on) }
func (s *systemFlag) IsBoolFlag() bool { return true }

func (s *systemFlag) Set(v string) error {
	on, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	s.on = on
	if !on {
		return nil
	}
	inst := s.fs.Lookup("installer")
	if inst == nil {
		return nil
	}
	if cur := inst.Value.String(); cur != "" && cur != systemInstaller {
		return fmt.Errorf("--system selects the %s installer; drop --installer %s", systemInstaller, cur)
	}
	return inst.Value.Set(systemInstaller)
}

// installed remembers which nodes the system installer finished, so the add hook can tell a skipped
// installer from a finished one.
var installed sync.Map

func init() {
	lifecycle.RegisterFlags("add", func(fs *flag.FlagSet) {
		fs.Var(&systemFlag{fs: fs}, "system", "install as a separated system node: agent user, one job user per slot, sudoers job-shim rules (needs root or passwordless sudo on the node)")
		fs.Int("slots", defaultSlots, "job users to create with --system (nself-ci-job-1..N)")
	})
	lifecycle.RegisterAgentInstaller(systemInstaller, systemInstall)
	lifecycle.RegisterAddHook(lifecycle.PointAdd, systemGuard)
}

// flagString reads a flag of the add command ("" when absent).
func flagString(fs *flag.FlagSet, name string) string {
	if fs == nil {
		return ""
	}
	if f := fs.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}

// systemGuard refuses an add that asked for --system but did not get it.
func systemGuard(_ context.Context, e lifecycle.HookEvent) error {
	if flagString(e.Flags, "system") != "true" {
		return nil
	}
	if _, ok := installed.LoadAndDelete(e.NodeID); !ok {
		return bootstrap.NewError(bootstrap.ReasonUsage, "--system needs --agent-binary and the %s installer; nothing was registered", systemInstaller)
	}
	return nil
}

// systemSeparation reports the node as uid-separated once the system layout is verified in place.
type systemSeparation struct {
	inner lifecycle.SeparationProber
	done  bool
}

func (s *systemSeparation) Detect(ctx context.Context, run func(context.Context, ...string) (string, error), p lifecycle.Platform) lifecycle.SeparationFacts {
	if s.done && p.OS == "linux" {
		return lifecycle.SeparationFacts{UIDSeparation: true, Reason: "system.users"}
	}
	return s.inner.Detect(ctx, run, p)
}

func systemInstall(ctx context.Context, d *lifecycle.Deps, r lifecycle.InstallRequest) error {
	slots, err := strconv.Atoi(flagString(r.Flags, "slots"))
	if err != nil || slots < 1 {
		slots = defaultSlots
	}
	if r.Source == nil {
		return bootstrap.NewError(bootstrap.ReasonUsage, "--system needs --agent-binary")
	}
	bin, cleanup, err := r.Source.Fetch(ctx, r.Platform)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	run := func(c context.Context, argv ...string) (string, error) { return d.Run(c, r.Target, argv...) }
	fam, keys, ok := pickFamily(ctx, d, r, run)
	if !ok {
		if r.Platform.OS == "darwin" {
			bootstrap.PrintDarwinHint(d.Stderr)
		}
		return bootstrap.NewError(bootstrap.ReasonUnsupported, "no system-install family for this node (tried %v)", keys)
	}
	opts := bootstrap.Options{Slots: slots, Run: run, AgentBinary: bin, Stdout: d.Stderr,
		Copy: func(c context.Context, local, remotePath string) error { return d.Copy(c, r.Target, local, remotePath) },
		Logf: func(f string, a ...any) { _, _ = fmt.Fprintf(d.Stderr, f+"\n", a...) }}
	if err = fam.Install(ctx, r.Target, opts); err != nil {
		return err
	}
	if err = linkAgent(ctx, run, r.Dir); err != nil {
		return err
	}
	d.Separation = &systemSeparation{inner: d.Separation, done: true}
	installed.Store(r.NodeID, true)
	return nil
}

// pickFamily detects the node's family; a darwin node uses the registered darwin family if any.
func pickFamily(ctx context.Context, _ *lifecycle.Deps, r lifecycle.InstallRequest, run bootstrap.RunFunc) (bootstrap.Family, []string, bool) {
	f, keys, ok, err := bootstrap.Detect(ctx, run)
	if err != nil {
		return nil, []string{bootstrap.Detail(err)}, false
	}
	return f, keys, ok
}

// linkAgent makes the user-mode path the lifecycle checks (`<dir>/bin/nself-ci-agent`) point at the
// system binary, so `nodes add` finds and versions the same agent the node serves.
func linkAgent(ctx context.Context, run bootstrap.RunFunc, dir string) error {
	bin := dir + "/bin"
	script := "set -eu\nmkdir -p " + remote.ShellQuote(bin) + "\nln -sf " + bootstrap.AgentPath + " " + remote.ShellQuote(bin+"/nself-ci-agent") + "\n"
	if _, err := run(ctx, "sh", "-c", script); err != nil {
		return bootstrap.NewError(bootstrap.ReasonStep, "bootstrap step link-agent failed: %s", bootstrap.Detail(err))
	}
	return nil
}
