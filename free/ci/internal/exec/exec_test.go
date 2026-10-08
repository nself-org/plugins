package exec

import (
	"context"
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

func TestTimeout(t *testing.T) {
	e := &Executor{Grace: 100 * time.Millisecond}
	r := e.Run(context.Background(), JobSpec{AttemptID: "timeout", CoordinatorID: "coord", Home: t.TempDir(), Timeout: 50 * time.Millisecond, Command: []string{"sh", "-c", "sleep 30"}})
	if r.Attempt.FailureClass != "timeout" {
		t.Fatalf("timeout: %+v", r)
	}
}
