package plan

// Purpose: static placeability of a job against the local world, decided at
// plan time (the run-time form of the same condition is E618).
// Inputs:  a candidate job, one matrix platform and the World.
// Outputs: an unplaceable.* reason with detail naming the missing item, or
// no reason when the job can run; for a catalogue check that is optional, a
// missing tool is advisory.tool_missing and the check is skipped instead.
// Constraints: order platform, docker, service, tool, label, resources; the
// first failure wins. With nodes or providers (World.Remote) only the
// platform is checked here; the scheduler judges the rest.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func needsDocker(c *cand) bool {
	j := c.job
	return j.Container != nil || j.Isolation == "container" || j.Isolation == "sandboxed-container" || len(j.NeedsServices) > 0
}

// unplaceable reports why entry platform cannot run c, or "" when it can.
func unplaceable(c *cand, platform string, w World, noMatch bool) (model.Reason, string) {
	if noMatch {
		return "unplaceable.platform", "no declared support platform matches the matrix"
	}
	if platform != "" && platform != w.Platform && !toSet(w.Runners)[platform] {
		return "unplaceable.platform", "platform " + platform + " has no registered runner"
	}
	if w.Remote {
		return "", ""
	}
	if needsDocker(c) && !w.Docker {
		return "unplaceable.docker", "Docker is not available"
	}
	if w.ServicesKnown {
		have := toSet(w.Services)
		for _, s := range c.job.NeedsServices {
			if !have[s] {
				return "unplaceable.service", "service " + s + " is not available"
			}
		}
	}
	// A job with its own container.image brings its own tools: the host (or the
	// pinned image) list says nothing about it (D-1027).
	if miss := missing(requiredTools(c), w.Tools); len(miss) > 0 && !ownImage(c) {
		return "unplaceable.tool", "tool " + strings.Join(miss, ", ") + " is not on PATH"
	}
	r := c.job.Requirements
	if r == nil {
		return "", ""
	}
	if miss := missing(r.Labels, w.Labels); len(miss) > 0 {
		return "unplaceable.label", "label " + strings.Join(miss, ", ") + " is not provided"
	}
	if miss := missing(r.Accelerators, w.Accelerators); len(miss) > 0 {
		return "unplaceable.resources", "accelerator " + strings.Join(miss, ", ") + " is not present"
	}
	if r.CPU > w.CPU {
		return "unplaceable.resources", fmt.Sprintf("needs %d cpu, host has %d", r.CPU, w.CPU)
	}
	if r.MemMB > w.MemMB {
		return "unplaceable.resources", fmt.Sprintf("needs %d MB memory, host has %d", r.MemMB, w.MemMB)
	}
	return "", ""
}

// ownImage is true when the job names its own container image.
func ownImage(c *cand) bool { return c.job.Container != nil && c.job.Container.Image != "" }

// advisoryToolMissing is true for an optional catalogue check whose tool is
// absent from a closed world: it is skipped, not unplaceable.
func advisoryToolMissing(c *cand, w World) bool {
	return c.derived && !c.required && !w.Remote && len(missing(requiredTools(c), w.Tools)) > 0 && !ownImage(c)
}

func requiredTools(c *cand) []string {
	var tools []string
	if c.tool != "" {
		tools = append(tools, c.tool)
	}
	if c.job.Requirements != nil {
		tools = append(tools, c.job.Requirements.Tools...)
	}
	return tools
}

// missing returns the sorted wanted items absent from have.
func missing(want, have []string) []string {
	if len(want) == 0 {
		return nil
	}
	set := toSet(have)
	var out []string
	for _, x := range sortedUnique(want) {
		if !set[x] {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
