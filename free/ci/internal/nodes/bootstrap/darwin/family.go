package darwin

// Purpose: the darwin bootstrap family (Epic D20, P7-NODE-31): `nodes add --system` on a Mac creates the
// hidden agent user and one job user per slot with dscl, a sudoers drop-in with one job-shim rule per
// slot user, the agent under /opt/nself-ci-agent, state under /var/db and the pull-mode LaunchDaemon;
// Uninstall removes all of it. It registers itself with the bootstrap family registry (P7-NODE-06).
// Inputs:  bootstrap.Options from `nodes add --system` over ssh, or from `nself-ci-agent uninstall
//          --system` run on the Mac itself (Run is then a local exec).
// Outputs: a node in the system layout; Logf lines per step; an E669 with a reason key on failure.
// Constraints: the ssh user must be root or have passwordless sudo; every step is idempotent; a re-run
//          with more slots adds only the missing users, with fewer slots retires a surplus user only when
//          it owns no file; no account joins admin or wheel and the agent user gets no sudo right of its
//          own; nothing here runs dscl on the machine that holds the coordinator (every command goes
//          through Options.Run).

import (
	"context"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

// Family is the darwin bootstrap family.
type Family struct {
	// XcodePathPrefix is put in front of the job user's PATH for the Xcode license probe; the tests of
	// the macOS leg point it at a stub xcodebuild. Empty in production.
	XcodePathPrefix string
}

func init() {
	if err := bootstrap.RegisterFamily(&Family{}); err != nil {
		panic(err)
	}
}

// Name implements bootstrap.Family.
func (*Family) Name() string { return "darwin" }

// Match implements bootstrap.Family: a macOS node is keyed "darwin" (bootstrap.OSKeys).
func (*Family) Match(os string) bool { return os == "darwin" }

var _ bootstrap.Family = (*Family)(nil)

// Install implements bootstrap.Family.
func (f *Family) Install(ctx context.Context, t remote.Target, o bootstrap.Options) error {
	o, err := bind(o, t, true)
	if err != nil {
		return err
	}
	if out, uerr := o.Run(ctx, "uname", "-s"); uerr != nil || lastLine(out) != "Darwin" {
		return bootstrap.NewError(bootstrap.ReasonUnsupported, "the darwin family installs on macOS nodes only")
	}
	el, err := newElevated(ctx, o)
	if err != nil {
		return err
	}
	steps := []struct{ reason, name, script string }{
		{ReasonUsers, "users", usersScript(o.Slots)},
		{bootstrap.ReasonStep, "directories", dirsScript(o.Slots)},
	}
	for _, s := range steps {
		if _, err = el.sh(ctx, s.reason, s.name, s.script); err != nil {
			return err
		}
		o.Logf("bootstrap %s: done", s.name)
	}
	if err = f.retireSurplus(ctx, el, o); err != nil {
		return err
	}
	if err = f.installSudoers(ctx, el, o); err != nil {
		return err
	}
	if err = f.installAgent(ctx, el, o); err != nil {
		return err
	}
	if err = f.installService(ctx, el, o); err != nil {
		return err
	}
	if _, err = el.sh(ctx, bootstrap.ReasonStep, "verify", verifyScript(o.Slots)); err != nil {
		return err
	}
	o.Logf("bootstrap verify: done")
	f.warnXcode(ctx, el, o)
	return nil
}

// Uninstall implements bootstrap.Family: it removes the service, the accounts, the sudoers drop-in and
// every directory Install created. Run it as root, or as a user with passwordless sudo.
func (f *Family) Uninstall(ctx context.Context, o bootstrap.Options) error {
	o, err := bind(o, o.Target, false)
	if err != nil {
		return err
	}
	el, err := newElevated(ctx, o)
	if err != nil {
		return err
	}
	if _, err = el.sh(ctx, ReasonLaunchd, "uninstall-service", bootoutScript()); err != nil {
		return err
	}
	if _, err = el.sh(ctx, ReasonUsers, "uninstall", uninstallScript()); err != nil {
		return err
	}
	o.Logf("bootstrap uninstall: done")
	return nil
}
