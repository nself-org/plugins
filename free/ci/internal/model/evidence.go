package model

// Evidence is the ci.evidence/v1 gate document. Freshness is computed at read time.
type Evidence struct {
	Schema               string               `json:"schema"`
	Revision             string               `json:"revision"`
	RepoID               string               `json:"repo_id"`
	RunID                string               `json:"run_id"`
	Pipeline             PipelineRef          `json:"pipeline"`
	Job                  *string              `json:"job,omitempty"`
	Attempt              *AttemptRef          `json:"attempt,omitempty"`
	Trigger              Trigger              `json:"trigger"`
	SourceTrust          TrustClass           `json:"source_trust"`
	PolicyDigest         string               `json:"policy_digest"`
	InputDigests         InputDigests         `json:"input_digests"`
	WorktreeState        WorktreeState        `json:"worktree_state"`
	TrackedChangesDigest *string              `json:"tracked_changes_digest"`
	Binding              Binding              `json:"binding"`
	Runner               RunnerRef            `json:"runner"`
	Producer             Producer             `json:"producer"`
	Timings              Timings              `json:"timings"`
	ToolVersions         map[string]string    `json:"tool_versions"`
	Checks               []Check              `json:"checks"`
	Artifacts            []Artifact           `json:"artifacts"`
	Retries              []Retry              `json:"retries"`
	Selection            Selection            `json:"selection"`
	PlacementRef         *string              `json:"placement_ref,omitempty"`
	Result               Result               `json:"result"`
	Counts               Counts               `json:"counts"`
	Freshness            Freshness            `json:"freshness"`
	Signature            Signature            `json:"signature"`
	ProviderAttestation  *ProviderAttestation `json:"provider_attestation,omitempty"`
}

// Pipeline is one planned run and its selected jobs.
type Pipeline struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Kind      PipelineKind `json:"kind"`
	Selection Selection    `json:"selection"`
	Jobs      []Job        `json:"jobs,omitempty"`
}

// PipelineRef identifies the pipeline in evidence.
type PipelineRef struct {
	ID   string       `json:"id"`
	Name string       `json:"name"`
	Kind PipelineKind `json:"kind"`
}

// Attempt is one execution of a job.
type Attempt struct {
	ID           string       `json:"id"`
	N            int          `json:"n"`
	State        JobState     `json:"state"`
	FailureClass FailureClass `json:"failure_class,omitempty"`
}

// AttemptRef identifies an attempt.
type AttemptRef struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

// Selection records why a pipeline was full or affected.
type Selection struct {
	Mode   SelectionMode `json:"mode"`
	Reason Reason        `json:"reason"`
	Digest string        `json:"digest"`
}

// DigestRef records the path and SHA256 of a bound input.
type DigestRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// InputDigests bind evidence to project inputs.
type InputDigests struct {
	Config    string      `json:"config"`
	Lockfiles []DigestRef `json:"lockfiles"`
	Generated []DigestRef `json:"generated"`
}

// Binding states whether evidence may gate its revision.
type Binding struct {
	State  BindingState   `json:"state"`
	Reason *BindingReason `json:"reason"`
}

// RunnerRef identifies the producing runner, without capability claims.
type RunnerRef struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Platform  string    `json:"platform"`
	Isolation Isolation `json:"isolation"`
}

// Producer identifies who recorded evidence.
type Producer struct {
	Kind    ProducerKind `json:"kind"`
	Version string       `json:"version"`
}

// Timings records the execution interval.
type Timings struct {
	QueuedAt   *string `json:"queued_at"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	DurationMS *int64  `json:"duration_ms"`
}

// Check is one gate check, including skipped work.
type Check struct {
	ID           string       `json:"id"`
	JobID        string       `json:"job_id"`
	Kind         string       `json:"kind"`
	Tool         string       `json:"tool"`
	ToolVersion  *string      `json:"tool_version"`
	Required     bool         `json:"required"`
	Substantive  bool         `json:"substantive"`
	AllowFailure bool         `json:"-"`
	Result       CheckResult  `json:"result"`
	Reason       Reason       `json:"reason,omitempty"`
	FailureClass FailureClass `json:"failure_class,omitempty"`
	Findings     int          `json:"findings"`
	Excerpt      string       `json:"excerpt"`
	LogRef       *string      `json:"log_ref"`
}

// Artifact is a committed output digest.
type Artifact struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Committed bool   `json:"committed"`
}

// Retry records a repeated attempt.
type Retry struct {
	JobID   string       `json:"job_id"`
	Attempt int          `json:"attempt"`
	Class   FailureClass `json:"class"`
}

// Counts is the check count summary.
type Counts struct {
	Pass  int `json:"pass"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
	Error int `json:"error"`
	Total int `json:"total"`
}

// Freshness is a read-time state, never persisted as a fact.
type Freshness struct {
	State  FreshnessState `json:"state"`
	Reason *string        `json:"reason"`
}

// Signature identifies the local record or runner signature.
type Signature struct {
	Kind  SignatureKind `json:"kind"`
	KeyID *string       `json:"key_id,omitempty"`
	Value *string       `json:"value,omitempty"`
}

// ProviderAttestation records a verified hosted provider statement.
type ProviderAttestation struct {
	Provider        string `json:"provider"`
	RunID           string `json:"run_id"`
	RunAttempt      int    `json:"run_attempt"`
	Ref             string `json:"ref"`
	WorkflowPath    string `json:"workflow_path"`
	WorkflowSHA     string `json:"workflow_sha"`
	WorkflowDigest  string `json:"workflow_digest"`
	TemplateVersion string `json:"template_version"`
	Nonce           string `json:"nonce"`
	ArtifactID      string `json:"artifact_id"`
	ArtifactSHA256  string `json:"artifact_sha256"`
	VerifiedAt      string `json:"verified_at"`
}

// Event is one ci.events/v1 stream record.
type Event struct {
	Schema     string         `json:"schema"`
	Seq        int64          `json:"seq"`
	TS         string         `json:"ts"`
	Type       EventType      `json:"type"`
	RunID      string         `json:"run_id"`
	PipelineID string         `json:"pipeline_id"`
	JobID      *string        `json:"job_id,omitempty"`
	AttemptID  *string        `json:"attempt_id,omitempty"`
	Data       map[string]any `json:"data"`
}
