//go:build integration

package sim

// Purpose: helpers shared by the node scenario legs (P7-NODE-17): process CPU sampling for the overhead
//          metrics and the capability shapes of the fixture nodes.
// Inputs:  none beyond the arguments.
// Outputs: durations, capability edits.
// Constraints: CPU is the whole test process (user plus system) from getrusage, so every figure that uses it
//          is an upper bound for the component under test and says so in its metric context.

import (
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// cpuTime is the user plus system CPU time this process has used so far.
func cpuTime() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// cpuPct is the CPU used between two cpuTime samples as a percentage of one core over wall.
func cpuPct(before, after, wall time.Duration) float64 {
	if wall <= 0 {
		return 0
	}
	return float64(after-before) / float64(wall) * 100
}

// shapePlatform sets the platform facts of a fixture node.
func shapePlatform(osName, arch string) func(*model.Capability) {
	return func(c *model.Capability) {
		c.Platform.OS = assigned(osName)
		c.Platform.Arch = assigned(arch)
	}
}

// shapeDiscovered marks a node as a discovered candidate that was never paired.
func shapeDiscovered(osName, arch string) func(*model.Capability) {
	p := shapePlatform(osName, arch)
	return func(c *model.Capability) {
		p(c)
		c.Lifecycle.Discovered, c.Lifecycle.Eligible = true, false
		c.Lifecycle.AuthorizedProjects = []string{}
	}
}

// shapeDeployHost marks a node as matched by the deploy inventory, with no override.
func shapeDeployHost(osName, arch string) func(*model.Capability) {
	p := shapePlatform(osName, arch)
	return func(c *model.Capability) {
		p(c)
		src := "inventory"
		c.DeployHost = model.CapabilityDeployHost{Matched: true, Source: &src, Override: false}
	}
}
