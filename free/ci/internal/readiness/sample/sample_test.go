package sample

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

// proc is a fake /proc: read N holds the successive /proc/stat contents.
type proc struct {
	files map[string]string
	stats []string
	reads int
}

func (p *proc) readFile(path string) ([]byte, error) {
	if path == "/proc/stat" {
		if p.reads >= len(p.stats) {
			return nil, errors.New("no more samples")
		}
		p.reads++
		return []byte(p.stats[p.reads-1]), nil
	}
	if s, ok := p.files[path]; ok {
		return []byte(s), nil
	}
	return nil, fs.ErrNotExist
}

func noPause(context.Context, time.Duration) error { return nil }

func disk(n uint64) func(string) (uint64, error) {
	return func(string) (uint64, error) { return n, nil }
}

func TestLinuxSample(t *testing.T) {
	p := &proc{
		files: map[string]string{
			"/proc/meminfo": "MemTotal:       16000000 kB\nMemFree:  100 kB\nMemAvailable:    8388608 kB\n",
			"/proc/loadavg": "46.00 30.10 20.00 3/900 1234\n",
		},
		// 1000 jiffies pass; 930 are idle (idle + iowait): 93% idle.
		stats: []string{"cpu  100 0 100 800 0 0 0 0 0 0\ncpu0 1 1 1 1\n", "cpu  140 0 130 1700 30 0 0 0 0 0\n"},
	}
	s := Sampler{OS: "linux", Cores: 8, ReadFile: p.readFile, FreeBytes: disk(5 << 30), DiskPath: "/ci", Pause: noPause}
	live := s.Sample(context.Background())
	if live.MemFreeMB == nil || *live.MemFreeMB != 8192 {
		t.Errorf("mem %v", live.MemFreeMB)
	}
	if live.LoadPerCore == nil || *live.LoadPerCore != 46.0/8 {
		t.Errorf("load %v", live.LoadPerCore)
	}
	if live.CPUIdlePct == nil || *live.CPUIdlePct != 93 {
		t.Errorf("idle %v", live.CPUIdlePct)
	}
	if live.DiskFreeMB == nil || *live.DiskFreeMB != 5120 {
		t.Errorf("disk %v", live.DiskFreeMB)
	}
	if len(live.Notes) != 0 {
		t.Errorf("notes %v", live.Notes)
	}
}

func TestLinuxUnreadableValuesStayNil(t *testing.T) {
	// Nothing readable: every /proc value is nil with a note, and the sample still returns.
	s := Sampler{OS: "linux", ReadFile: (&proc{}).readFile, FreeBytes: func(string) (uint64, error) { return 0, errors.New("statfs denied") }, DiskPath: "/ci", Pause: noPause}
	live := s.Sample(context.Background())
	if live.MemFreeMB != nil || live.LoadPerCore != nil || live.CPUIdlePct != nil || live.DiskFreeMB != nil {
		t.Fatalf("no value may be invented: %+v", live)
	}
	for _, name := range []string{"mem_free_mb", "load_per_core", "cpu_idle_pct", "disk_free_mb"} {
		if live.Notes[name] == "" {
			t.Errorf("no note for %s", name)
		}
	}
	// A /proc/stat that does not advance gives no idle figure rather than 0% or 100%.
	same := "cpu  1 0 1 1 0 0 0 0 0 0\n"
	p := &proc{files: map[string]string{}, stats: []string{same, same}}
	if live = (Sampler{OS: "linux", ReadFile: p.readFile, Pause: noPause, FreeBytes: disk(1)}).Sample(context.Background()); live.CPUIdlePct != nil {
		t.Fatalf("idle %v", *live.CPUIdlePct)
	}
	// Garbage in /proc/meminfo and /proc/loadavg.
	p = &proc{files: map[string]string{"/proc/meminfo": "MemAvailable: lots kB\n", "/proc/loadavg": "x y z"}, stats: []string{same, same}}
	if live = (Sampler{OS: "linux", ReadFile: p.readFile, Pause: noPause, FreeBytes: disk(1)}).Sample(context.Background()); live.MemFreeMB != nil || live.LoadPerCore != nil {
		t.Fatalf("garbage must stay nil: %+v", live)
	}
}

func TestLinuxIdleHonoursCancellation(t *testing.T) {
	p := &proc{stats: []string{"cpu  1 0 1 1 0 0 0 0 0 0\n"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	live := (Sampler{OS: "linux", ReadFile: p.readFile, FreeBytes: disk(1)}).Sample(ctx)
	if live.CPUIdlePct != nil || live.Notes["cpu_idle_pct"] == "" {
		t.Fatalf("%+v", live)
	}
}

const vmStat = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               25600.
Pages active:                            200000.
Pages inactive:                           12800.
Pages speculative:                         1600.
Pages wired down:                         90000.
`

const topOut = "Processes: 500 total\nLoad Avg: 2.1, 2.0, 2.0\nCPU usage: 3.5% user, 5.0% sys, 91.5% idle\nSharedLibs: 1M\n"

func fakeMac(calls *[][]string) Runner {
	return RunnerFunc(func(_ context.Context, argv []string, d time.Duration) (Result, error) {
		if calls != nil {
			*calls = append(*calls, argv)
		}
		if d <= 0 {
			return Result{}, errors.New("a command needs a timeout")
		}
		switch argv[0] {
		case "/usr/bin/vm_stat":
			return Result{Stdout: vmStat}, nil
		case "/usr/sbin/sysctl":
			return Result{Stdout: "{ 9.00 8.50 8.00 }\n"}, nil
		case "/usr/bin/top":
			return Result{Stdout: topOut}, nil
		case "/bin/ps":
			return Result{Stdout: "launchd\nobs\n  \nSafari\n"}, nil
		}
		return Result{ExitCode: 127}, nil
	})
}

func TestDarwinSample(t *testing.T) {
	var calls [][]string
	s := Sampler{OS: "darwin", Cores: 8, Run: fakeMac(&calls), FreeBytes: disk(1 << 30), DiskPath: "/ci"}
	live := s.Sample(context.Background())
	// (25600 + 12800 + 1600) pages of 16 KiB = 640000 KiB = 625 MiB.
	if live.MemFreeMB == nil || *live.MemFreeMB != 625 {
		t.Errorf("mem %v", live.MemFreeMB)
	}
	if live.LoadPerCore == nil || *live.LoadPerCore != 9.0/8 {
		t.Errorf("load %v", live.LoadPerCore)
	}
	if live.CPUIdlePct == nil || *live.CPUIdlePct != 91.5 {
		t.Errorf("idle %v", live.CPUIdlePct)
	}
	for _, c := range calls {
		if c[0][0] != '/' {
			t.Errorf("%v: macOS probes use absolute binary paths", c)
		}
	}
	names, err := s.Processes(context.Background())
	if err != nil || len(names) != 3 || names[0] != "launchd" || names[1] != "obs" || names[2] != "Safari" {
		t.Fatalf("processes %v %v", names, err)
	}
}

func TestDarwinCommandFailureLeavesOnlyThatValueNil(t *testing.T) {
	run := RunnerFunc(func(ctx context.Context, argv []string, d time.Duration) (Result, error) {
		if argv[0] == "/usr/bin/top" {
			return Result{ExitCode: 1}, nil
		}
		if argv[0] == "/usr/sbin/sysctl" {
			return Result{}, errors.New("timed out after 5s")
		}
		return fakeMac(nil).Run(ctx, argv, d)
	})
	live := (Sampler{OS: "darwin", Cores: 2, Run: run, FreeBytes: disk(1)}).Sample(context.Background())
	if live.MemFreeMB == nil || live.CPUIdlePct != nil || live.LoadPerCore != nil {
		t.Fatalf("%+v", live)
	}
	if live.Notes["cpu_idle_pct"] != "top exited 1" || live.Notes["load_per_core"] != "timed out after 5s" {
		t.Fatalf("notes %v", live.Notes)
	}
}

func TestOtherPlatformsReadNothing(t *testing.T) {
	s := Sampler{OS: "plan9", FreeBytes: disk(2 << 20)}
	live := s.Sample(context.Background())
	if live.MemFreeMB != nil || live.LoadPerCore != nil || live.CPUIdlePct != nil || live.DiskFreeMB == nil || *live.DiskFreeMB != 2 {
		t.Fatalf("%+v", live)
	}
	if _, err := s.Processes(context.Background()); err == nil {
		t.Fatal("process names are not readable on plan9")
	}
}

func TestLinuxProcesses(t *testing.T) {
	m := fstest.MapFS{
		"proc/1/comm":    {Data: []byte("systemd\n")},
		"proc/22/comm":   {Data: []byte("zoom.us\n")},
		"proc/33":        {Mode: fs.ModeDir}, // exited before comm was read
		"proc/self/comm": {Data: []byte("ignored\n")},
		"proc/stat":      {Data: []byte("cpu 1\n")},
	}
	s := Sampler{OS: "linux",
		ReadDir:  func(string) ([]fs.DirEntry, error) { return fs.ReadDir(m, "proc") },
		ReadFile: func(p string) ([]byte, error) { return fs.ReadFile(m, p[1:]) }}
	names, err := s.Processes(context.Background())
	if err != nil || len(names) != 2 || names[0] != "systemd" || names[1] != "zoom.us" {
		t.Fatalf("%v %v", names, err)
	}
}

// Jobs run under <CI home>/work, so the disk floor measures the volume that holds the CI home
// (nearest existing parent on a fresh host), not the login home.
func TestDiskDefaultsToTheCIHome(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("NSELF_CI_HOME", filepath.Join(parent, "not", "yet", "created"))
	var asked string
	s := Sampler{OS: "plan9", FreeBytes: func(p string) (uint64, error) { asked = p; return 3 << 20, nil }}
	if live := s.Sample(context.Background()); live.DiskFreeMB == nil || asked != parent {
		t.Fatalf("asked %q live %+v", asked, live)
	}
	t.Setenv("NSELF_CI_HOME", "")
	t.Setenv("HOME", parent)
	if s.Sample(context.Background()); asked != parent {
		t.Fatalf("the default CI home ~/.nself/ci falls back to the nearest existing parent: %q", asked)
	}
}

func TestRealStatfsReadsSomething(t *testing.T) {
	live := Sampler{OS: "plan9", DiskPath: t.TempDir()}.Sample(context.Background())
	if live.DiskFreeMB == nil || *live.DiskFreeMB < 0 {
		t.Fatalf("statfs of a temp dir gave %+v (%v)", live.DiskFreeMB, live.Notes)
	}
}

// An incomplete process list would let a yield_to target hide in the cut tail, so a truncated
// `ps` is an error, never a shorter list.
func TestDarwinTruncatedProcessListIsAnError(t *testing.T) {
	run := RunnerFunc(func(context.Context, []string, time.Duration) (Result, error) {
		return Result{Stdout: "launchd\n", Truncated: true}, nil
	})
	if names, err := (Sampler{OS: "darwin", Run: run}).Processes(context.Background()); err == nil {
		t.Fatalf("truncated list returned %v", names)
	}
}
