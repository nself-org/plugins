// Package sample reads the live facts a host needs to answer "may I take CI work now?":
// free memory, CPU idle percentage, load average, free disk and the running process names.
//
// Purpose: P7-AUTO-08 readiness. The CI-32 capacity probe and NODE-22 already return the
// static facts (cores, total memory, battery); this package adds only what changes from
// second to second.
// Inputs: a Sampler whose readers default to the real host: Linux /proc, macOS `vm_stat`,
// `sysctl` and `top -l 1` run through the exec funnel (Funnel), free disk through statfs.
// Outputs: Live, where a value that could not be read is nil (never a made-up default).
// Constraints: no shell, no whole environment, every command has a timeout; tests inject
// every reader and never touch the real /proc or sysctl; a reader failure leaves that
// one value nil and records why in Live.Notes, it never fails the whole sample.
package sample

import (
	"context"
	"io/fs"
	"os"
	"runtime"
	"time"
)

// Result is what one command printed and how it ended.
type Result struct {
	ExitCode int
	Stdout   string
	// Truncated is true when the command printed more than MaxOutput bytes and the rest was
	// dropped. A caller that needs the whole output (a process list) must treat it as an error.
	Truncated bool
}

// Runner runs one argv with a timeout, without a shell. The error is a start failure,
// a timeout or a cancellation; a non-zero exit is a Result, not an error.
type Runner interface {
	Run(ctx context.Context, argv []string, timeout time.Duration) (Result, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(ctx context.Context, argv []string, timeout time.Duration) (Result, error)

// Run implements Runner.
func (f RunnerFunc) Run(ctx context.Context, argv []string, timeout time.Duration) (Result, error) {
	return f(ctx, argv, timeout)
}

// Live is one reading. A nil pointer means "could not be read on this host".
type Live struct {
	MemFreeMB   *int64
	LoadPerCore *float64
	CPUIdlePct  *float64
	DiskFreeMB  *int64
	// Notes says, per value name, why it is nil.
	Notes map[string]string
}

// Sampler reads Live from a host. The zero value reads the real one.
type Sampler struct {
	// OS is runtime.GOOS unless a test sets it.
	OS string
	// Cores divides the load average; below 1 it is read as 1.
	Cores int
	// DiskPath is the directory whose filesystem is measured; empty means the home directory.
	DiskPath string
	// ReadFile and ReadDir read /proc on Linux; nil uses the os package.
	ReadFile func(string) ([]byte, error)
	ReadDir  func(string) ([]fs.DirEntry, error)
	// Run runs `vm_stat`, `sysctl`, `top` and `ps` on macOS; nil uses the exec funnel.
	Run Runner
	// FreeBytes returns the bytes a normal user can still write under a path; nil uses statfs.
	FreeBytes func(path string) (uint64, error)
	// Pause waits between the two /proc/stat reads of the Linux idle sample; nil sleeps.
	Pause func(ctx context.Context, d time.Duration) error
}

func (s Sampler) goos() string {
	if s.OS != "" {
		return s.OS
	}
	return runtime.GOOS
}

func (s Sampler) readFile(path string) ([]byte, error) {
	if s.ReadFile != nil {
		return s.ReadFile(path)
	}
	return os.ReadFile(path)
}

func (s Sampler) readDir(path string) ([]fs.DirEntry, error) {
	if s.ReadDir != nil {
		return s.ReadDir(path)
	}
	return os.ReadDir(path)
}

func (s Sampler) runner() Runner {
	if s.Run != nil {
		return s.Run
	}
	return Funnel{}
}

func (s Sampler) cores() float64 {
	if s.Cores < 1 {
		return 1
	}
	return float64(s.Cores)
}

// Sample reads every live value this host can give.
func (s Sampler) Sample(ctx context.Context) Live {
	live := Live{Notes: map[string]string{}}
	switch s.goos() {
	case "linux":
		s.sampleLinux(ctx, &live)
	case "darwin":
		s.sampleDarwin(ctx, &live)
	default:
		for _, name := range []string{"mem_free_mb", "load_per_core", "cpu_idle_pct"} {
			live.Notes[name] = "not readable on " + s.goos()
		}
	}
	s.sampleDisk(&live)
	return live
}

// note records why a value is nil.
func (l *Live) note(name string, err error) {
	if err != nil {
		l.Notes[name] = err.Error()
	}
}

func i64(v int64) *int64     { return &v }
func f64(v float64) *float64 { return &v }
