package main

// Purpose: `nself-ci-agent uninstall --system` (P7-NODE-31, Epic D20): remove the system install of this
// node. It is the bootstrap family's Uninstall, run on the node itself: the launchd service, the hidden
// agent user, the job users and their groups, the sudoers drop-in and the agent, state, work, job and
// log directories.
// Inputs:  `--system` (required: it names what is removed). Run it as root (`sudo nself-ci-agent
//          uninstall --system`): the accounts and the sudoers file belong to root.
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
	_ "github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin" // registers the darwin family
)

func init() {
	must(RegisterSubcommand("uninstall", "remove the system install of this node (--system; run as root)", runUninstall))
}

// localTools are the only programs the family runs on this machine, each by absolute path: the process
// is root, so nothing is looked up through PATH.
var localTools = map[string]string{"id": "/usr/bin/id", "sh": "/bin/sh", "uname": "/usr/bin/uname", "sudo": "/usr/bin/sudo"}

// localRun runs one fixed argv on this machine and returns its combined output. It uses os.StartProcess:
// os/exec stays inside internal/exec (architecture rule).
func localRun(ctx context.Context, argv ...string) (string, error) {
	bin, ok := localTools[argv[0]]
	if !ok {
		return "", errors.New("uninstall runs no program named " + argv[0])
	}
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer r.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		w.Close()
		return "", err
	}
	defer null.Close()
	proc, err := os.StartProcess(bin, append([]string{bin}, argv[1:]...), &os.ProcAttr{Files: []*os.File{null, w, w}})
	w.Close()
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
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent uninstall --system   (run as root)")
		return 2
	}
	f, ok := bootstrap.Lookup(runtime.GOOS)
	if !ok {
		fmt.Fprintln(sio.Stderr, bootstrap.NewError("uninstall_unsupported", "no system install exists for %s; nothing to remove", runtime.GOOS))
		return 1
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(sio.Stderr, "error: uninstall --system must run as root: sudo nself-ci-agent uninstall --system")
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
