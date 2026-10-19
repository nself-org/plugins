package bootstrap

// Purpose: the Linux families (apt, dnf, apk): the toolchain manifest of each package manager and the
// Install and Uninstall sequence they share. P7-NODE-06 registers them from init; macOS is
// P7-NODE-31's family, registered through the same registry.
// Inputs:  Options from `nodes add --system`.
// Outputs: a node with the agent user `nself-ci`, one job user per slot, one sudoers rule per slot
//          user, the agent under /opt/nself-ci-agent, the state and job directories and (systemd
//          families) the pull-mode unit.
// Constraints: the ssh user must be root or have passwordless sudo (E669 no_root otherwise); every step
//          is idempotent; a re-run with more slots adds only the missing users and rules; a re-run with
//          fewer slots retires a surplus user only when it owns no file (its rule goes either way);
//          no user is put in the docker group and the agent user gets no sudo right of its own.

import (
	"context"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// pkgFamily is one package manager's manifest.
type pkgFamily struct {
	name    string
	ids     []string // /etc/os-release ID and ID_LIKE values this family handles
	install string   // the non-interactive install command, packages appended
	pkgs    []string // the toolchain manifest: sudo (the job shim), shadow tools, git, curl, tar, CA bundle
	systemd bool     // writes the pull-mode unit
}

// linuxFamilies are the three Linux families.
func linuxFamilies() []Family {
	return []Family{
		&pkgFamily{name: "apt", ids: []string{"debian", "ubuntu", "raspbian", "linuxmint", "pop", "kali"},
			install: "export DEBIAN_FRONTEND=noninteractive; apt-get update -qq && apt-get install -y -qq --no-install-recommends",
			pkgs:    []string{"sudo", "passwd", "git", "curl", "ca-certificates", "tar", "findutils"}, systemd: true},
		&pkgFamily{name: "dnf", ids: []string{"fedora", "rhel", "centos", "rocky", "almalinux", "amzn", "ol"},
			install: "dnf install -y -q --setopt=install_weak_deps=False",
			pkgs:    []string{"sudo", "shadow-utils", "git", "curl", "ca-certificates", "tar", "findutils"}, systemd: true},
		&pkgFamily{name: "apk", ids: []string{"alpine"},
			install: "apk add --no-cache -q",
			pkgs:    []string{"sudo", "shadow", "git", "curl", "ca-certificates", "tar", "findutils"}},
	}
}

// Name implements Family.
func (f *pkgFamily) Name() string { return f.name }

// Match implements Family.
func (f *pkgFamily) Match(os string) bool {
	for _, id := range f.ids {
		if id == os {
			return true
		}
	}
	return false
}

// Packages is the toolchain manifest of the family.
func (f *pkgFamily) Packages() []string { return append([]string(nil), f.pkgs...) }

// elevated runs a script as root: directly when the ssh user is root, through `sudo -n` otherwise.
type elevated struct {
	o    Options
	root bool
}

func newElevated(ctx context.Context, o Options) (*elevated, error) {
	out, err := o.Run(ctx, "id", "-u")
	if err != nil {
		return nil, NewError(ReasonStep, "cannot reach the node: %s", Detail(err))
	}
	if lastLine(out) == "0" {
		return &elevated{o: o, root: true}, nil
	}
	if _, err = o.Run(ctx, "sudo", "-n", "true"); err != nil {
		return nil, NewError(ReasonNoRoot, "the ssh user is not root and has no passwordless sudo; --system needs one of the two")
	}
	return &elevated{o: o}, nil
}

func (e *elevated) sh(ctx context.Context, step, script string) (string, error) {
	argv := []string{"sh", "-c", script}
	if !e.root {
		argv = append([]string{"sudo", "-n"}, argv...)
	}
	out, err := e.o.Run(ctx, argv...)
	if err != nil {
		return out, stepErr(step, err)
	}
	return out, nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

// Install implements Family.
func (f *pkgFamily) Install(ctx context.Context, t remote.Target, o Options) error {
	o, err := o.bind(t, true)
	if err != nil {
		return err
	}
	if out, uerr := o.Run(ctx, "uname", "-s"); uerr != nil || lastLine(out) != "Linux" {
		return NewError(ReasonUnsupported, "the %s family installs on Linux nodes only", f.name)
	}
	el, err := newElevated(ctx, o)
	if err != nil {
		return err
	}
	steps := []struct{ name, script string }{
		{"packages", packagesScript(f.install, f.pkgs)},
		{"users", usersScript(o.Slots)},
		{"directories", dirsScript(o.Slots)},
		{"sudoers", sudoersScript(o.Slots)},
	}
	if f.systemd {
		steps = append(steps, struct{ name, script string }{"unit", unitScript()})
	}
	for _, s := range steps {
		if _, err = el.sh(ctx, s.name, s.script); err != nil {
			return err
		}
		o.Logf("bootstrap %s: done", s.name)
	}
	if err = f.retireSurplus(ctx, el, o); err != nil {
		return err
	}
	if err = f.installAgent(ctx, el, o); err != nil {
		return err
	}
	_, err = el.sh(ctx, "verify", verifyScript(o.Slots))
	return err
}

// installAgent uploads and installs the binary when the caller gave one.
func (f *pkgFamily) installAgent(ctx context.Context, el *elevated, o Options) error {
	if o.AgentBinary == "" {
		return nil
	}
	const uploaded = ".nself-ci-agent.upload"
	if err := o.Copy(ctx, o.AgentBinary, uploaded); err != nil {
		return NewError(ReasonStep, "bootstrap step agent-upload failed: %s", Detail(err))
	}
	if _, err := el.sh(ctx, "agent", agentScript(uploaded)); err != nil {
		return err
	}
	o.Logf("bootstrap agent: installed %s", AgentPath)
	return nil
}

// Uninstall implements Family.
func (f *pkgFamily) Uninstall(ctx context.Context, o Options) error {
	o, err := o.bind(o.Target, false)
	if err != nil {
		return err
	}
	el, err := newElevated(ctx, o)
	if err != nil {
		return err
	}
	if _, err = el.sh(ctx, "uninstall", uninstallScript()); err != nil {
		return err
	}
	o.Logf("bootstrap uninstall: done")
	return nil
}
