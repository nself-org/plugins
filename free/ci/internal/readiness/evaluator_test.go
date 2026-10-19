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
		Config:   func(context.Context) (Config, error) { return cfg, nil },
		Sampler:  linuxHost(t, "8388608", "46.0"),
		Hook:     hook,
		Cores:    func() int { return 8 },
		CacheTTL: -1,
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

func TestEvaluatorWithoutConfigRefuses(t *testing.T) {
	if _, err := (&Evaluator{}).Evaluate(context.Background()); err == nil {
		t.Fatal("a missing Config source would drop the operator's hook and yield_to; it must be an error")
	}
}

func TestEvaluatorCachesForTheTTLAndWhenConfigChanges(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cfg := DefaultConfig()
	probes := 0
	hook := sample.RunnerFunc(func(context.Context, []string, time.Duration) (sample.Result, error) {
		probes++
		return sample.Result{}, nil
	})
	cfg.Cmd = []string{"/opt/busy"}
	ev := &Evaluator{
		Config:   func(context.Context) (Config, error) { return cfg, nil },
		Sampler:  linuxHost(t, "8388608", "0.1"),
		Hook:     hook,
		Cores:    func() int { return 8 },
		CacheTTL: 5 * time.Second,
		Now:      func() time.Time { return now },
	}
	for i := 0; i < 3; i++ {
		if r, err := ev.Evaluate(context.Background()); err != nil || !r.Ready {
			t.Fatalf("%+v %v", r, err)
		}
	}
	if probes != 1 {
		t.Fatalf("three calls inside the TTL must probe once, probed %d times", probes)
	}
	cfg.Cmd = []string{"/opt/other"} // an operator edit takes effect at once
	_, _ = ev.Evaluate(context.Background())
	now = now.Add(6 * time.Second) // the TTL passed
	_, _ = ev.Evaluate(context.Background())
	if probes != 3 {
		t.Fatalf("a config change and an expired TTL must each probe again, probed %d times", probes)
	}
}

func TestEvaluatorHandsOutCopiesAndHonoursContext(t *testing.T) {
	cfg := DefaultConfig()
	ev := &Evaluator{Config: func(context.Context) (Config, error) { return cfg, nil }, Sampler: linuxHost(t, "100", "0.1"), Cores: func() int { return 8 }}
	first, err := ev.Evaluate(context.Background())
	if err != nil || first.Ready {
		t.Fatalf("%+v %v", first, err)
	}
	first.Why[0], first.Why = "tampered", append(first.Why, "extra")
	*first.MemFreeMB = 999999
	second, _ := ev.Evaluate(context.Background())
	if second.Why[0] == "tampered" || len(second.Why) != 1 || *second.MemFreeMB == 999999 {
		t.Fatalf("a caller edited the cached answer: %+v", second)
	}
	// A caller queued behind a running probe leaves when its own context ends.
	ev.sem <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ev.Evaluate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	<-ev.sem
}
