package probe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// collect runs each literal command separately; optional command failures are unknown facts.
func collect(ctx context.Context, c *model.Capability, runner commandRunner, expectedOS string, now time.Time, local bool) error {
	uname, err := runner(ctx, cmdUname)
	if err != nil {
		return fmt.Errorf("uname probe: %w", err)
	}
	osName, arch, kernel := parseUname(uname)
	if osName == nil {
		return fmt.Errorf("unsupported or malformed uname output")
	}
	if expectedOS != "" && expectedOS != *osName {
		return fmt.Errorf("host OS changed during probe")
	}
	c.Platform.OS = observed(osName, now)
	c.Platform.Arch = observed(arch, now)
	c.Platform.Kernel = observed(kernel, now)
	c.Platform.Virtualization = observed[string](nil, now)
	commands := linuxCommands
	if *osName == "darwin" {
		commands = darwinCommands
	}
	outputs := make(map[string]string, len(commands))
	for _, command := range commands {
		if command == cmdUname {
			continue
		}
		out, runErr := runner(ctx, command)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if runErr == nil || command == cmdTools && out != "" {
			outputs[command] = strings.TrimSpace(out)
		}
	}
	c.Platform.OSVersion = observed(parseOSRelease(outputs[cmdOSRelease]), now)
	c.Resources.DiskFreeMB = observed(parseDisk(outputs[cmdDisk]), now)
	if !local {
		c.Resources.CPU = observed(parseNproc(outputs[cmdNproc]), now)
	}
	applyToolFacts(c, outputs, *osName, now)
	// This is an offered isolation ceiling. Registry.UpdateCapability may only lower it.
	ceiling := model.Isolation("process")
	if c.Tools.Docker.Value != nil && *c.Tools.Docker.Value {
		ceiling = "container"
	}
	if c.Tools.GVisor.Value != nil && *c.Tools.GVisor.Value {
		ceiling = "sandboxed-container"
	}
	c.Trust.Isolation = value(ceiling, now)
	c.Trust.Isolation.Source = "assigned"
	return nil
}
