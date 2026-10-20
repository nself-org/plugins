package exec

// Purpose: SpawnDetached starts a process that outlives its caller (P7-CI-63,
// Epic D30): `nself ci --detach` re-executes the plugin binary through it. The
// child gets a new session, stdin from /dev/null, stdout and stderr appended to
// one 0600 file and no other descriptor.
// Inputs:  argv (never a shell string), the full child environment, the log path.
// Outputs: the child pid; Detached.Exited closes when the child has ended.
// Constraints: this package is the only os/exec importer; the unix-only session
// call lives in detach_unix.go behind applyDetach, with a windows stub that
// returns E648 reason spawn (Epic D35); no shell sits between parent and child.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// detachCode is E648, declared by internal/detach (which sits above this package).
const detachCode = "E648"

// Detached is a started detached child.
type Detached struct {
	PID int
	// Exited closes when the child process has ended. While the caller lives it
	// also reaps the child; after the caller exits the child is re-parented.
	Exited <-chan struct{}
}

// SpawnDetached starts argv in a new session and returns its pid. env is the
// whole child environment; the log file is created 0600 (its directory 0700).
func SpawnDetached(argv, env []string, logPath string) (int, error) {
	d, err := StartDetached(argv, env, logPath)
	if err != nil {
		return 0, err
	}
	return d.PID, nil
}

// StartDetached is SpawnDetached that also reports when the child ends, so a
// caller can tell a child that died early from one that is still starting.
func StartDetached(argv, env []string, logPath string) (*Detached, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, coded(detachCode, "spawn: no command to start")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, coded(detachCode, "spawn: "+err.Error())
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, coded(detachCode, "spawn: "+err.Error())
	}
	defer func() { _ = logFile.Close() }()
	if err = os.Chmod(logPath, 0o600); err != nil {
		return nil, coded(detachCode, "spawn: "+err.Error())
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		return nil, coded(detachCode, "spawn: "+err.Error())
	}
	defer func() { _ = null.Close() }()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, logFile, logFile
	cmd.ExtraFiles = nil
	if err = applyDetach(cmd); err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, &Error{Code: detachCode, Message: fmt.Sprintf("spawn: %v", err), Err: err}
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	return &Detached{PID: cmd.Process.Pid, Exited: exited}, nil
}
