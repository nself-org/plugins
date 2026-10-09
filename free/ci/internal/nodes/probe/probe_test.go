package probe

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func setupProbe(t *testing.T, ids ...string) *registry.Registry {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	r := registry.New(s)
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		var c model.Capability
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		c.Identity.ID = id
		if _, err := r.Register(context.Background(), c, "operator"); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func fixtureRunner(ctx context.Context, command string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	switch command {
	case cmdUname:
		return "Linux 6.1.0 x86_64", nil
	case cmdOSRelease:
		return "ID=debian\nVERSION_ID=12", nil
	case cmdNproc:
		return "4", nil
	case cmdDisk:
		return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/root 204800 102400 102400 50% /", nil
	case cmdTools:
		return "/usr/bin/go\n/usr/bin/python3", errors.New("some tools missing")
	case cmdPython:
		return "Python 3.11.2", nil
	default:
		return "", errors.New("absent")
	}
}

func TestProbeParsers(t *testing.T) {
	tests := []struct {
		name                     string
		parse                    func(string) bool
		present, absent, garbage string
	}{
		{"uname", func(s string) bool { a, b, c := parseUname(s); return a != nil && b != nil && c != nil }, "Linux 6.1.0 x86_64", "", "nonsense"},
		{"os-release", func(s string) bool { return parseOSRelease(s) != nil }, "ID=debian\nVERSION_ID=12", "ID=debian", "VERSION_ID=nope"},
		{"nproc", func(s string) bool { return parseNproc(s) != nil }, "8", "", "-20"},
		{"df", func(s string) bool { return parseDisk(s) != nil }, "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/root 100 50 50 50% /", "", "garbage"},
		{"command-v", func(s string) bool { return parseTools(s)["docker"] }, "/usr/bin/docker", "", "docker"},
		{"docker-version", func(s string) bool { return parseVersion(s) != nil }, "27.1.0", "", "garbage"},
		{"xcode", func(s string) bool { return parseXcode(s, "/Applications/Xcode.app/Contents/Developer") != nil }, "Xcode 16.2\nBuild version 16C5032a", "", "garbage"},
		{"xcode-select", func(s string) bool { return parseXcode("Xcode 16.2", s) != nil }, "/Applications/Xcode.app/Contents/Developer", "", "garbage"},
		{"swift", func(s string) bool { return parseVersion(s) != nil }, "Apple Swift version 6.0.1", "", "garbage"},
		{"python3", func(s string) bool { return parseVersion(s) != nil }, "Python 3.11.2", "", "garbage"},
		{"tart", func(s string) bool { return parseVersion(s) != nil }, "tart 2.13.0", "", "garbage"},
		{"nvidia-smi", func(s string) bool { return parseNvidia(s) != nil }, "GPU 0: NVIDIA A100 (UUID: GPU-123)", "", "garbage"},
		{"system-profiler", func(s string) bool { return parseDisplays(s) != nil }, `{"SPDisplaysDataType":[{"_name":"Apple M4"}]}`, "", "garbage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.parse(tc.present) || tc.parse(tc.absent) || tc.parse(tc.garbage) {
				t.Fatalf("parser %s did not distinguish present, absent, garbage", tc.name)
			}
		})
	}
}

func TestProbeFixedReadOnlyCommands(t *testing.T) {
	r := setupProbe(t, "01J00000000000000000000000")
	var seen []string
	p := &SSHProber{Registry: r, runner: func(ctx context.Context, command string) (string, error) {
		seen = append(seen, command)
		return fixtureRunner(ctx, command)
	}}
	if _, err := p.Probe(context.Background(), "01J00000000000000000000000"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, linuxCommands) {
		t.Fatalf("probe command set changed: %q", seen)
	}
	for _, command := range seen {
		if strings.ContainsAny(command, ";|`$\\\n") {
			t.Fatalf("shell control in command: %q", command)
		}
	}
}

func TestProbeLocalCapacityAndUnknown(t *testing.T) {
	r := setupProbe(t, "01J00000000000000000000000")
	var calls atomic.Int32
	p := &LocalProber{Registry: r, runner: fixtureRunner, capacity: func() ciexec.Capacity {
		calls.Add(1)
		return ciexec.Capacity{CPUs: 4, MemoryMB: 2048, BatteryPercent: -1}
	}}
	n, err := p.Probe(context.Background(), "01J00000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || *n.Capability.Resources.CPU.Value != 4 || *n.Capability.Resources.MemMB.Value != 2048 {
		t.Fatal("capacity not composed exactly once")
	}
	if n.Capability.Availability.BatteryPct.Value != nil || n.Capability.Availability.PluggedIn.Value != nil || n.Capability.Availability.BatteryPct.Confidence != "unknown" {
		t.Fatal("no power_supply must be unknown")
	}
	if n.Capability.Resources.Accelerators.Value != nil || n.Capability.Resources.Accelerators.Confidence != "unknown" {
		t.Fatal("absent nvidia-smi must be unknown")
	}
	if _, err := p.Probe(context.Background(), "01J00000000000000000000000"); err != nil || calls.Load() != 1 {
		t.Fatal("60s store cache failed", err)
	}
	other := &LocalProber{Registry: r, runner: func(context.Context, string) (string, error) {
		t.Fatal("cache did not survive prober instance")
		return "", nil
	}}
	if _, err := other.Probe(context.Background(), "01J00000000000000000000000"); err != nil {
		t.Fatal(err)
	}
}

func TestProbeConcurrentLimits(t *testing.T) {
	ids := make([]string, 20)
	for i := range ids {
		ids[i] = "01J000000000000000000000" + string(rune('A'+i))
	}
	r := setupProbe(t, ids...)
	var active, max, calls atomic.Int32
	runner := func(ctx context.Context, command string) (string, error) {
		if command != cmdUname {
			return fixtureRunner(ctx, command)
		}
		n := active.Add(1)
		for {
			old := max.Load()
			if n <= old || max.CompareAndSwap(old, n) {
				break
			}
		}
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
		return fixtureRunner(ctx, command)
	}
	p := &SSHProber{Registry: r, runner: runner}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := p.Probe(context.Background(), id); err != nil {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	if max.Load() > 8 || max.Load() < 2 || calls.Load() != 20 {
		t.Fatalf("20 hosts: max=%d calls=%d", max.Load(), calls.Load())
	}
	// Eight callers for one host share a single stored probe.
	max.Store(0)
	calls.Store(0)
	id := "01J00000000000000000000000"
	r = setupProbe(t, id)
	p.Registry = r
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Probe(context.Background(), id); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 || max.Load() != 1 {
		t.Fatalf("same-host probes not serialized: calls=%d max=%d", calls.Load(), max.Load())
	}
}

func TestProbeDarwinFactsAndIsolation(t *testing.T) {
	r := setupProbe(t, "01J00000000000000000000000")
	n, err := r.Get(context.Background(), "01J00000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	container := model.Isolation("container")
	n.Capability.Trust.Isolation = model.Fact[model.Isolation]{Value: &container, Source: "assigned", ObservedAt: time.Now().UTC(), Confidence: "known"}
	// The registry is the authority; a probe cannot set trust directly.
	admin, err := store.NewAdminAuthority("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.SetTrust(context.Background(), n.Record.ID, n.Capability.Trust, admin); err != nil {
		t.Fatal(err)
	}
	runner := func(ctx context.Context, cmd string) (string, error) {
		switch cmd {
		case cmdUname:
			return "Darwin 24.1.0 arm64", nil
		case cmdTools:
			return "/usr/local/bin/runsc\n/opt/homebrew/bin/tart", nil
		case cmdXcode:
			return "Xcode 16.2\nBuild version 16C5032a", nil
		case cmdXcodePath:
			return "/Applications/Xcode.app/Contents/Developer", nil
		case cmdSwift:
			return "Apple Swift version 6.0.1", nil
		case cmdPython:
			return "Python 3.11.2", nil
		case cmdTart:
			return "tart 2.13.0", nil
		case cmdDisplays:
			return `{"SPDisplaysDataType":[{"_name":"Apple M4"}]}`, nil
		default:
			return "", errors.New("absent")
		}
	}
	p := &SSHProber{Registry: r, runner: runner}
	n, err = p.Probe(context.Background(), n.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := n.Capability.Tools.Toolchains; !reflect.DeepEqual(*got["xcode"].Value, "16.2") || got["swift"].Value == nil || got["python3"].Value == nil {
		t.Fatal("darwin toolchains missing", got)
	}
	if n.Capability.Tools.Tart.Value == nil || !*n.Capability.Tools.Tart.Value || n.Capability.Resources.Accelerators.Value == nil {
		t.Fatal("darwin tools missing")
	}
	if *n.Capability.Trust.Isolation.Value != "container" {
		t.Fatal("gvisor raised registered isolation")
	}
	if !strings.Contains(strings.Join(*n.Capability.Resources.Accelerators.Value, ","), "gpu") {
		t.Fatal("GPU kind missing")
	}
}
