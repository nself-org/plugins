package readiness

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
)

func linuxHost(t *testing.T, memKB, load string) sample.Sampler {
	t.Helper()
	m := fstest.MapFS{
		"proc/meminfo": {Data: []byte("MemAvailable: " + memKB + " kB\n")},
		"proc/loadavg": {Data: []byte(load + " 1 2 3/4 5\n")},
		"proc/stat":    {Data: []byte("cpu  1 0 1 1 0 0 0 0 0 0\n")},
	}
	reads := 0
	return sample.Sampler{OS: "linux",
		ReadFile: func(p string) ([]byte, error) {
			if p == "/proc/stat" { // 90% idle between the two reads
				reads++
				if reads%2 == 1 {
					return []byte("cpu  10 0 0 90 0 0 0 0 0 0\n"), nil
				}
				return []byte("cpu  20 0 0 180 0 0 0 0 0 0\n"), nil
			}
			return fs.ReadFile(m, p[1:])
		},
		ReadDir:   func(string) ([]fs.DirEntry, error) { return nil, nil },
		FreeBytes: func(string) (uint64, error) { return 100 << 30, nil },
		Pause:     func(context.Context, time.Duration) error { return nil },
	}
}

func TestEvaluatorEndToEndOnLinux(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Cmd = []string{"/opt/busy"}
	hook := runnerOf(sample.Result{ExitCode: 3, Stdout: "nightly backup running"}, nil, nil, nil)
	ev := Evaluator{
		Config:  func(context.Context) (Config, error) { return cfg, nil },
		Sampler: linuxHost(t, "8388608", "46.0"),
		Hook:    hook,
		Cores:   func() int { return 8 },
	}
	r, err := ev.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Ready || len(r.Why) != 1 || r.Why[0] != "nightly backup running" {
		t.Fatalf("only the hook refuses (load 46 on 8 cores at 90%% idle is fine): %+v", r)
	}
	if r.MemFreeMB == nil || *r.MemFreeMB != 8192 || r.LoadPerCore == nil || *r.LoadPerCore != 5.75 || r.CPUIdlePct == nil || *r.CPUIdlePct != 90 {
		t.Fatalf("facts %+v", r)
	}
	cfg.Cmd = nil
	if r, _ = ev.Evaluate(context.Background()); !r.Ready {
		t.Fatalf("without the hook the host is ready: %+v", r)
	}
	ev.Sampler = linuxHost(t, "100", "0.1")
	if r, _ = ev.Evaluate(context.Background()); r.Ready || !strings.Contains(r.Why[0], "free memory 0 MB") {
		t.Fatalf("low memory refuses: %+v", r)
	}
}

func TestEvaluatorConfigErrorFailsTheEvaluation(t *testing.T) {
	boom := errors.New("E672 readiness.min_mem_mb: bad")
	ev := Evaluator{Config: func(context.Context) (Config, error) { return Config{}, boom }}
	if _, err := ev.Evaluate(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
}
