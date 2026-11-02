package run

// Purpose: decide whether the daemon warm-up is worth starting, so a run that never reaches a container
// runtime does not spend a `podman info` / `docker info` spawn on its start path (P7-CI-46).
// Inputs:  the plan and the options.
// Outputs: Coordinator.mayAskRuntime.
// Constraints: it only gates the prefetch of the `info` answer. Container admission still probes in
// Registry.Select, and the orphan sweep asks the runtime whenever it needs it, so a false answer never
// weakens a check.

// mayAskRuntime reports whether this run is likely to ask the container runtime: --container, a source
// requested as untrusted, or a job above process isolation.
func (c *Coordinator) mayAskRuntime() bool {
	if c.Opts.Container || c.Opts.Trust == "untrusted" {
		return true
	}
	for _, pj := range c.Plan.Jobs {
		switch pj.Job.Isolation {
		case "", "process":
		default:
			return true
		}
		if pj.Job.Container != nil {
			return true
		}
	}
	return false
}
