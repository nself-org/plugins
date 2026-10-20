package main

// Purpose: `nself-ci-agent uninstall --system` (P7-NODE-31, Epic D20): remove the system install of this
// node. It is the bootstrap family's Uninstall, run on the node itself: the launchd service, the hidden
// agent user, the job users and their groups, the sudoers drop-in and the agent, state, work, job and
// log directories.
// Inputs:  `--system` (required: it names what is removed). Run it as root from the root-owned copy
//          (`sudo /Library/PrivilegedHelperTools/org.nself.ci-agent uninstall --system`): the accounts
//          and the sudoers file belong to root, and the installed agent belongs to the agent user, so
//          a root shell must not execute it (the command refuses when the binary or any directory
//          above it is not root-owned and closed to group and others).
// Outputs: one line per step on stderr; exit 0 removed, 1 failed, 2 usage.
// Constraints: only a family registered for this machine's GOOS acts (today darwin); no family is E669
//          reason uninstall_unsupported; it starts no job, so the PreStart production guard does not
//          apply (see TestEveryJobRunningSubcommandRunsThePreStartGuard).

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin" // registers the darwin family
)

func init() {
	must(RegisterSubcommand("uninstall", "remove the system install of this node (--system; run as root)", runUninstall))
}

// The only programs the family asks this machine to run (root checks `id -u`, then `sh -c <script>`),
// each by absolute path: the process is root, so nothing is looked up through PATH.
const (
	idBin = "/usr/bin/id"
	shBin = "/bin/sh"
)

// localRun runs one fixed argv on this machine and returns its combined output. It uses os.StartProcess
// with a constant program (TestSingleSSHExecSite) because os/exec stays inside internal/exec.
func localRun(ctx context.Context, argv ...string) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	null, err := os.Open(os.DevNull)
	if err != nil {
		_ = w.Close()
		return "", err
	}
	defer func() { _ = null.Close() }()
	attr := &os.ProcAttr{Files: []*os.File{null, w, w}, Env: []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}}
	var proc *os.Process
	switch argv[0] {
	case "id":
		proc, err = os.StartProcess(idBin, append([]string{idBin}, argv[1:]...), attr)
	case "sh":
		proc, err = os.StartProcess(shBin, append([]string{shBin}, argv[1:]...), attr)
	default:
		err = errors.New("uninstall runs no program named " + argv[0])
	}
	_ = w.Close()
	if err != nil {
		return "", err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = proc.Kill()
		case <-done:
		}
	}()
	var out bytes.Buffer
	_, _ = out.ReadFrom(r)
	st, werr := proc.Wait()
	close(done)
	if werr != nil {
		return out.String(), werr
	}
	if !st.Success() {
		return out.String(), errors.New(st.String())
	}
	return out.String(), nil
}

func runUninstall(ctx context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	system := fs.Bool("system", false, "remove the system install (users, sudoers rules, service, directories)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || !*system {
		fmt.Fprintf(sio.Stderr, "usage: sudo %s uninstall --system   (the root-owned copy of the agent)\n", darwin.TrustedPath)
		return 2
	}
	f, ok := bootstrap.Lookup(runtime.GOOS)
	if !ok {
		fmt.Fprintln(sio.Stderr, bootstrap.NewError("uninstall_unsupported", "no system install exists for %s; nothing to remove", runtime.GOOS))
		return 1
	}
	if os.Geteuid() != 0 {
		fmt.Fprintf(sio.Stderr, "error: uninstall --system must run as root: sudo %s uninstall --system\n", darwin.TrustedPath)
		return 1
	}
	if exe, err := os.Executable(); err != nil || trustedRootPath(exe) != nil {
		why := "the running binary"
		if err == nil {
			why = trustedRootPath(exe).Error()
		}
		fmt.Fprintf(sio.Stderr, "error: refusing to run as root from a file the agent user can change (%s).\nRun the root-owned copy: sudo %s uninstall --system\n", why, darwin.TrustedPath)
		return 1
	}
	o := bootstrap.Options{Run: localRun, Stdout: sio.Stdout,
		Logf: func(format string, a ...any) { fmt.Fprintf(sio.Stderr, format+"\n", a...) }}
	if err := f.Uninstall(ctx, o); err != nil {
		fmt.Fprintln(sio.Stderr, err)
		return 1
	}
	return 0
}
