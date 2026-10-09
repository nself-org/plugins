package probe

import (
	"context"
	"os/exec"
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
	// The caller selects command solely from the constants in commands.go.
	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	return string(out), err
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
