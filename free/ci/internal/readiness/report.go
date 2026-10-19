package readiness

// Purpose: contract:ci.readiness v1 (schema free/ci/schemas/readiness.v1.schema.json),
// the answer to "may this host take CI work now?" that the scheduler, the watcher and
// the demand file (P7-AUTO-09) all read.
// Outputs: Report. A value that could not be read is null, never a guessed number.
// Constraints: additive changes only; Why and YieldedTo are always arrays.

// SchemaID is the value of Report.Schema.
const SchemaID = "ci.readiness/v1"

// clone returns a Report that shares no slice or pointer with r, so a caller that edits its
// copy cannot change a cached answer.
func (r Report) clone() Report {
	c := r
	c.Why = append([]string{}, r.Why...)
	c.YieldedTo = append([]string{}, r.YieldedTo...)
	if r.MemFreeMB != nil {
		v := *r.MemFreeMB
		c.MemFreeMB = &v
	}
	if r.LoadPerCore != nil {
		v := *r.LoadPerCore
		c.LoadPerCore = &v
	}
	if r.CPUIdlePct != nil {
		v := *r.CPUIdlePct
		c.CPUIdlePct = &v
	}
	if r.DiskFreeMB != nil {
		v := *r.DiskFreeMB
		c.DiskFreeMB = &v
	}
	return c
}

// Report is one readiness decision with the facts behind it.
type Report struct {
	Schema      string   `json:"schema"`
	Ready       bool     `json:"ready"`
	Why         []string `json:"why"`
	MemFreeMB   *int64   `json:"mem_free_mb"`
	LoadPerCore *float64 `json:"load_per_core"`
	CPUIdlePct  *float64 `json:"cpu_idle_pct"`
	DiskFreeMB  *int64   `json:"disk_free_mb"`
	YieldedTo   []string `json:"yielded_to"`
}
