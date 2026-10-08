//go:build linux

package exec

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDoubleForkBeforeCancelKillsAdoptedChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child int
	script := `import os,signal,time
r,w=os.pipe()
p=os.fork()
if p==0:
    os.close(r)
    q=os.fork()
    if q==0:
        os.setsid()
        signal.signal(signal.SIGTERM,signal.SIG_IGN)
        os.write(w,str(os.getpid()).encode())
        time.sleep(30)
        os._exit(0)
    os._exit(0)
os.close(w)
os.waitpid(p,0)
print(os.read(r,32).decode(),flush=True)
time.sleep(30)`
	r := (&Executor{Grace: 100 * time.Millisecond}).Run(ctx, JobSpec{AttemptID: "double-fork", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"python3", "-c", script}, Output: func(p []byte) {
		if n, err := strconv.Atoi(strings.TrimSpace(string(p))); err == nil {
			child = n
			cancel()
		}
	}})
	if child > 0 {
		defer func() { _ = syscall.Kill(child, syscall.SIGKILL) }()
	}
	if child <= 0 || r.Attempt.FailureClass != "cancelled" {
		t.Fatalf("run=%+v child=%d", r, child)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for syscall.Kill(child, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if syscall.Kill(child, 0) == nil {
		t.Fatalf("adopted grandchild %d survived", child)
	}
}
