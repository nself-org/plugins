package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type splitRedactor struct{ held []byte }

func (r *splitRedactor) Redact(p []byte) []byte {
	r.held = append(r.held, p...)
	if len(r.held) < 6 {
		return nil
	}
	out := strings.ReplaceAll(string(r.held), "secret", "[redacted]")
	r.held = nil
	return []byte(out)
}
func (r *splitRedactor) Flush() []byte {
	out := append([]byte(nil), r.held...)
	r.held = nil
	return out
}

type countingRedactor struct{ calls int }

func (r *countingRedactor) Redact(p []byte) []byte { r.calls++; return p }
func (r *countingRedactor) Flush() []byte          { return nil }

func TestOutputFloodPassesRedactor(t *testing.T) {
	r := &countingRedactor{}
	count := 0
	e := &Executor{Redactor: r}
	result := e.Run(context.Background(), JobSpec{AttemptID: "flood", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", "yes abc | head -c 1000000"}, Output: func(p []byte) { count += len(p) }})
	if result.Err != nil || result.ExitCode != 0 || count != 1000000 || r.calls < 2 {
		t.Fatalf("flood result=%+v bytes=%d redactor calls=%d", result, count, r.calls)
	}
}

func TestRedactorSplitAndWorkspaceCleanup(t *testing.T) {
	home := t.TempDir()
	var mu sync.Mutex
	var output []byte
	e := &Executor{Redactor: &splitRedactor{}}
	r := e.Run(context.Background(), JobSpec{AttemptID: "redact", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", "printf sec; sleep .1; printf ret"}, Output: func(p []byte) { mu.Lock(); defer mu.Unlock(); output = append(output, p...) }})
	if r.Err != nil || r.ExitCode != 0 {
		t.Fatalf("run: %+v", r)
	}
	if strings.Contains(string(output), "secret") || !strings.Contains(string(output), "[redacted]") {
		t.Fatalf("output: %q", output)
	}
	if _, err := os.Stat(filepath.Join(home, "work", "redact")); !os.IsNotExist(err) {
		t.Fatalf("workspace not removed: %v", err)
	}
}

func TestAdmissionBatteryAndQuota(t *testing.T) {
	job := model.Job{Path: "deep"}
	c := Capacity{CPUs: 2, MemoryMB: 2048, BatteryPercent: 20, Interactive: true}
	if d := Admit(job, Background, c, AdmissionPolicy{}); d.Allowed || d.Reason != "admission.battery" {
		t.Fatalf("battery: %+v", d)
	}
	if d := Admit(job, Foreground, c, AdmissionPolicy{}); !d.Allowed || d.Slots != 1 {
		t.Fatalf("foreground: %+v", d)
	}
	if d := Admit(job, Background, c, AdmissionPolicy{Force: true}); !d.Allowed {
		t.Fatalf("force: %+v", d)
	}
	c.CI = true
	if d := Admit(job, Background, c, AdmissionPolicy{Force: true}); !d.Allowed || d.ReserveCPUs != 0 || !d.ReservationsDisabled {
		t.Fatalf("CI: %+v", d)
	}
}

func TestRejectWorkdirEscape(t *testing.T) {
	e := &Executor{}
	r := e.Run(context.Background(), JobSpec{AttemptID: "escape", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Workdir: "../../.."}, Command: []string{"sh", "-c", "true"}})
	if r.Err == nil {
		t.Fatal("escaped workspace accepted")
	}
}

func TestDotWorkdirUsesWorkspace(t *testing.T) {
	for _, dir := range []string{"", "."} {
		t.Run(fmt.Sprintf("workdir=%q", dir), func(t *testing.T) {
			var output []byte
			result := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "dot-workdir", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Workdir: dir}, Command: []string{"sh", "-c", "pwd"}, Output: func(p []byte) { output = append(output, p...) }})
			if result.Err != nil || !strings.Contains(string(output), "/work/dot-workdir") {
				t.Fatalf("run=%+v output=%q", result, output)
			}
		})
	}
}

func TestAbsoluteWorkdirRejected(t *testing.T) {
	for _, dir := range []string{"/tmp", "../outside"} {
		r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "workdir-escape", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Workdir: dir}, Command: []string{"true"}})
		if r.Err == nil {
			t.Fatalf("accepted workdir %q", dir)
		}
	}
}

func TestSharedRedactorWaitRespectsCancellation(t *testing.T) {
	e := &Executor{Redactor: &splitRedactor{}}
	home := t.TempDir()
	firstDone := make(chan AttemptResult, 1)
	started := make(chan struct{}, 1)
	go func() {
		firstDone <- e.Run(context.Background(), JobSpec{AttemptID: "first", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", "echo started; sleep 2"}, Output: func([]byte) {
			select {
			case started <- struct{}{}:
			default:
			}
		}})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first run did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan AttemptResult, 1)
	go func() {
		secondDone <- e.Run(ctx, JobSpec{AttemptID: "second", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", "true"}})
	}()
	cancel()
	select {
	case r := <-secondDone:
		if r.Err != context.Canceled {
			t.Fatalf("waiting run=%+v", r)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("waiting run ignored cancellation")
	}
	<-firstDone
}

func TestTimeout(t *testing.T) {
	e := &Executor{Grace: 100 * time.Millisecond}
	r := e.Run(context.Background(), JobSpec{AttemptID: "timeout", CoordinatorID: "coord", Home: t.TempDir(), Timeout: 50 * time.Millisecond, Command: []string{"sh", "-c", "sleep 30"}})
	if r.Attempt.FailureClass != "timeout" {
		t.Fatalf("timeout: %+v", r)
	}
}

func TestForegroundLowMemoryGuaranteedSlot(t *testing.T) {
	d := Admit(model.Job{}, Foreground, Capacity{CPUs: 2, MemoryMB: 2048, FreeMemoryMB: 128, Interactive: true}, AdmissionPolicy{})
	if !d.Allowed || d.Slots != 1 {
		t.Fatalf("foreground slot lost: %+v", d)
	}
}

func TestPerRunRedactorFactory(t *testing.T) {
	var mu sync.Mutex
	count := 0
	e := &Executor{NewRedactor: func() Redactor { mu.Lock(); count++; mu.Unlock(); return &splitRedactor{} }}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var out []byte
			r := e.Run(context.Background(), JobSpec{AttemptID: fmt.Sprintf("factory-%d", i), CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", "printf sec; sleep .1; printf ret"}, Output: func(p []byte) { out = append(out, p...) }})
			if r.Err != nil || strings.Contains(string(out), "secret") || !strings.Contains(string(out), "[redacted]") {
				t.Errorf("run=%+v output=%q", r, out)
			}
		}(i)
	}
	wg.Wait()
	if count != 2 {
		t.Fatalf("redactors=%d", count)
	}
}

func TestWorkspaceCleanupFailureIsInfra(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "work", "cleanup")
	r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "cleanup", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", "mkdir trapped; echo secret > trapped/file; chmod 000 trapped"}})
	if _, err := os.Stat(workspace); err == nil {
		defer func() { _ = os.Chmod(filepath.Join(workspace, "trapped"), 0700); _ = os.RemoveAll(workspace) }()
		if r.Err == nil || r.Attempt.FailureClass != "infra" || r.Attempt.State == "passed" {
			t.Fatalf("cleanup silently passed: %+v", r)
		}
	} else if r.Err != nil {
		t.Fatalf("cleanup succeeded but run failed: %+v", r)
	}
}
