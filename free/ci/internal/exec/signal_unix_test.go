//go:build !windows

package exec

import (
	"context"
	"testing"
	"time"
)

// A job killed by a signal reports the signal's name, so diagnosis can tell a
// crash (SIGSEGV) from an external kill (SIGKILL) (D-1048).
func TestSignalNamedOnKilledJob(t *testing.T) {
	for _, c := range []struct{ script, want string }{
		{"kill -SEGV $$", "SIGSEGV"},
		{"kill -ABRT $$", "SIGABRT"},
		{"kill -KILL $$", "SIGKILL"},
		{"kill -TERM $$", "SIGTERM"},
	} {
		e := &Executor{Grace: 100 * time.Millisecond}
		r := e.Run(context.Background(), JobSpec{AttemptID: "sig", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", c.script}})
		if r.ExitCode != -1 || r.Signal != c.want {
			t.Errorf("%s: exit %d signal %q, want -1 %q (%+v)", c.script, r.ExitCode, r.Signal, c.want, r)
		}
	}
}

func TestSignalEmptyOnPlainExit(t *testing.T) {
	e := &Executor{Grace: 100 * time.Millisecond}
	r := e.Run(context.Background(), JobSpec{AttemptID: "plain", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", "exit 3"}})
	if r.ExitCode != 3 || r.Signal != "" {
		t.Fatalf("plain exit: %+v", r)
	}
}
