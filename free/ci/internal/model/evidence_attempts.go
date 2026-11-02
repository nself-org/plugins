package model

// Additive, optional ci.evidence/v1 keys (P7-AUTO-21). A document written
// before them has none of these keys and still validates; the contract stays
// v1 and experimental.

// RunnerKinds are the placement kinds an attempt can record.
// "unknown" is an attempt the coordinator admitted on a runner it knows only by id.
var RunnerKinds = []string{"local", "ssh", "agent", "hosted", "unknown"}

// AttemptRunner is where one attempt ran, from its recorded placement.
type AttemptRunner struct {
	Kind string `json:"kind"` // local | ssh | agent | hosted
	Tier string `json:"tier"`
	Pool string `json:"pool"`
	Rank int    `json:"rank"` // position of the chosen runner in the placement ranking (0 is first)
	// Node, Provider, Platform and Isolation name the runner that actually ran the attempt
	// (additive keys: a document written before them has none). Node is the runner id of the
	// placement, Platform is "os/arch", Isolation is the level the runner declares.
	Node      string    `json:"node,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Platform  string    `json:"platform,omitempty"`
	Isolation Isolation `json:"isolation,omitempty"`
}

// AttemptTimings splits an attempt's wall time from its state transitions.
// A nil member is a phase that did not happen (a local attempt has no transfer).
type AttemptTimings struct {
	QueueMS    *int64 `json:"queue_ms,omitempty"`
	TransferMS *int64 `json:"transfer_ms,omitempty"`
	RunMS      *int64 `json:"run_ms,omitempty"`
}

// AttemptEvidence is the runner and timing record of one attempt of a job.
type AttemptEvidence struct {
	JobID   string          `json:"job_id"`
	Attempt int             `json:"attempt"`
	Runner  AttemptRunner   `json:"runner"`
	Timings *AttemptTimings `json:"timings,omitempty"`
}

// EvidenceError is a coded infrastructure error met while producing the run.
type EvidenceError struct {
	Code    string `json:"code"`
	JobID   string `json:"job_id,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Detail  string `json:"detail,omitempty"` // short and already redacted
}
