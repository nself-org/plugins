package exec

import (
	"context"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sync"
	"time"
)

// runProcess returns the exit status and, when a signal ended the job, its name.
func (e *Executor) runProcess(ctx context.Context, s JobSpec, workspace string, env []string) (int, string, error) {
	defer removeProcessMarker(s.Home, s.AttemptID)
	resolvedRoot, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return -1, "", err
	}
	workdir := resolvedRoot
	if s.Job.Workdir != "" && s.Job.Workdir != "." {
		workdir = filepath.Join(workspace, s.Job.Workdir)
		rel, err := filepath.Rel(resolvedRoot, workdir)
		if err != nil || rel == ".." || len(rel) > 3 && rel[:3] == "../" {
			return -1, "", coded("E612", "workdir escapes workspace")
		}
		resolved, err := filepath.EvalSymlinks(workdir)
		if err != nil {
			return -1, "", err
		}
		if resolved != filepath.Join(resolvedRoot, rel) {
			return -1, "", coded("E612", "workdir symlink is forbidden")
		}
		workdir = resolved
	}
	env = append(env, "NSELF_CI_EXEC_ATTEMPT="+s.AttemptID)
	return e.runCommandSig(ctx, s.Stdin, limitedCommand(s), workdir, env, s.Output, nil, func(pid int) error {
		return writeProcessMarker(s.Home, s.AttemptID, s.CoordinatorID, pid)
	}, s.AttemptID)
}

func (e *Executor) runCommand(ctx context.Context, argv []string, dir string, env []string, sink func([]byte), onCancel func(), onStart func(int) error, attempt string) (int, error) {
	return e.runCommandIn(ctx, nil, argv, dir, env, sink, onCancel, onStart, attempt)
}

// runCommandIn is runCommand with stdin fed from a reader (nil keeps /dev/null).
func (e *Executor) runCommandIn(ctx context.Context, stdin io.Reader, argv []string, dir string, env []string, sink func([]byte), onCancel func(), onStart func(int) error, attempt string) (int, error) {
	code, _, err := e.runCommandSig(ctx, stdin, argv, dir, env, sink, onCancel, onStart, attempt)
	return code, err
}

// runCommandSig is runCommandIn that also names the signal that ended the
// command (SIGSEGV, SIGKILL, ...; "" when it exited on its own).
func (e *Executor) runCommandSig(ctx context.Context, stdin io.Reader, argv []string, dir string, env []string, sink func([]byte), onCancel func(), onStart func(int) error, attempt string) (int, string, error) {
	return e.runCommandFiles(ctx, stdin, nil, argv, dir, env, sink, onCancel, onStart, attempt)
}

// runCommandFiles is runCommandSig that also hands the child extra open files (fd 3 and up). The
// parent's copies are closed once the child has started, or on a failed start.
func (e *Executor) runCommandFiles(ctx context.Context, stdin io.Reader, extra []*os.File, argv []string, dir string, env []string, sink func([]byte), onCancel func(), onStart func(int) error, attempt string) (int, string, error) {
	closeExtra := func() {
		for _, f := range extra {
			_ = f.Close()
		}
	}
	cmd := osexec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.ExtraFiles = extra
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if err := prepareProcess(cmd); err != nil {
		closeExtra()
		return -1, "", err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		closeExtra()
		return -1, "", err
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err = cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		closeExtra()
		return -1, "", err
	}
	_ = writer.Close()
	closeExtra()
	if onStart != nil {
		if err := onStart(cmd.Process.Pid); err != nil {
			_ = killGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			_ = reader.Close()
			return -1, "", err
		}
	}
	tracker := trackDescendants(cmd.Process.Pid, attempt)
	defer tracker.stop()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = reader.Close() }()
		buf := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				chunk := e.Redactor.Redact(buf[:n])
				if len(chunk) > 0 && sink != nil {
					sink(append([]byte(nil), chunk...))
				}
			}
			if err != nil {
				return
			}
		}
	}()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var runErr error
	select {
	case runErr = <-wait:
	case <-ctx.Done():
		if onCancel != nil {
			go onCancel()
		}
		tracker.snapshot()
		_ = terminateGroup(cmd.Process.Pid)
		tracker.signalTerminate()
		deadline := time.Now().Add(e.Grace)
		leaderDone := false
		timer := time.NewTimer(e.Grace)
		select {
		case runErr = <-wait:
			leaderDone = true
		case <-timer.C:
		}
		timer.Stop()
		tracker.snapshot()
		// The leader is gone: give the rest of the group the grace period to follow it, and stop waiting as
		// soon as none is left (an exited orphan is reaped here so it does not count as alive).
		for leaderDone && time.Now().Before(deadline) {
			tracker.reap()
			if !groupAlive(cmd.Process.Pid) && !tracker.anyAlive() {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = killGroup(cmd.Process.Pid)
		tracker.signalKill()
		if !leaderDone {
			select {
			case runErr = <-wait:
			case <-time.After(2 * time.Second):
				runErr = ctx.Err()
			}
		}
	}
	tracker.snapshot()
	tracker.signalKill()
	_ = killGroup(cmd.Process.Pid)
	// Give the reader a short chance to drain output after the leader exits.
	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	select {
	case <-drained:
	case <-time.After(100 * time.Millisecond):
		_ = reader.Close()
		<-drained
	}
	if tail := e.Redactor.Flush(); len(tail) > 0 && sink != nil {
		sink(tail)
	}
	if ctx.Err() != nil {
		return -1, "", ctx.Err()
	}
	if exit, ok := runErr.(*osexec.ExitError); ok {
		return exit.ExitCode(), exitSignal(exit), nil
	}
	if runErr != nil {
		return -1, "", runErr
	}
	return 0, "", nil
}
