package probe

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

// LocalProber composes the CI executor's capacity observation exactly once.
type LocalProber struct {
	Registry *registry.Registry
	runner   commandRunner
	capacity func() ciexec.Capacity
}

func (p *LocalProber) Probe(ctx context.Context, id string) (registry.Node, error) {
	return run(ctx, p.Registry, id, func(ctx context.Context, c *model.Capability) error {
		runner := p.runner
		if runner == nil {
			runner = localCommand
		}
		capacity := p.capacity
		if capacity == nil {
			capacity = ciexec.Probe
		}
		now := time.Now().UTC()
		applyCapacity(c, capacity(), now)
		return collect(ctx, c, runner, "", now, true)
	})
}

func localCommand(ctx context.Context, command string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("local POSIX probe is unavailable on Windows")
	}
	if !fixedCommand(command) {
		return "", fmt.Errorf("probe command is not in the fixed read-only set")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	proc, err := startLocalProcess(ctx, func() (*os.Process, error) {
		return os.StartProcess("/bin/sh", []string{"sh", "-c", command}, &os.ProcAttr{
			Files: []*os.File{nil, w, w},
			Env:   []string{"PATH=" + os.Getenv("PATH"), "LANG=C"},
		})
	})
	_ = w.Close()
	if err != nil {
		return "", err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = proc.Kill()
		case <-done:
		}
	}()
	out, readErr := io.ReadAll(io.LimitReader(r, 64<<10))
	if _, err := io.Copy(io.Discard, r); readErr == nil {
		readErr = err
	}
	state, waitErr := proc.Wait()
	close(done)
	if ctx.Err() != nil {
		return string(out), ctx.Err()
	}
	if readErr != nil {
		return string(out), readErr
	}
	if waitErr != nil {
		return string(out), waitErr
	}
	if !state.Success() {
		return string(out), fmt.Errorf("probe command exited %s", state.String())
	}
	return string(out), nil
}

func startLocalProcess(ctx context.Context, start func() (*os.Process, error)) (*os.Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return start()
}

func applyCapacity(c *model.Capability, cap ciexec.Capacity, now time.Time) {
	if cap.CPUs > 0 {
		c.Resources.CPU = value(float64(cap.CPUs), now)
	} else {
		c.Resources.CPU = observed[float64](nil, now)
	}
	if cap.MemoryMB > 0 {
		c.Resources.MemMB = value(int64(cap.MemoryMB), now)
	} else {
		c.Resources.MemMB = observed[int64](nil, now)
	}
	if cap.BatteryPercent >= 0 && cap.BatteryPercent <= 100 {
		c.Availability.BatteryPct = value(cap.BatteryPercent, now)
		c.Availability.PluggedIn = value(cap.PluggedIn, now)
	} else {
		c.Availability.BatteryPct = observed[int](nil, now)
		c.Availability.PluggedIn = observed[bool](nil, now)
	}
	interactive := "idle"
	if cap.Interactive {
		interactive = "active"
	}
	c.Availability.Interactive = value(interactive, now)
}
