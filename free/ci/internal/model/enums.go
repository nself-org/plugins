package model

// Closed v1 value lists are shared by the model and schema generator.
type Reason string
type JobState string
type FailureClass string
type TrustClass string
type Isolation string
type NetworkScope string
type SecretClass string
type PrivacyZone string
type SelectionMode string
type CheckResult string
type Result string
type EventType string
type WorktreeState string
type BindingState string
type BindingReason string
type FreshnessState string
type ProducerKind string
type SignatureKind string
type Trigger string
type PipelineKind string
type JobKind string
type Preset string
type Priority string
type JobPath string
type ArtifactKind string
type ArtifactVisibility string

var enumValues = map[string][]string{
	"Reason":             {"pipeline.excluded", "selection.no_base", "selection.full_required", "selection.affected", "when.branch", "when.paths", "when.trigger", "when.env_missing", "dep.failed", "fail_fast.cancelled", "unplaceable.platform", "unplaceable.docker", "unplaceable.tool", "unplaceable.service", "unplaceable.label", "unplaceable.resources", "admission.battery", "admission.reservation", "advisory.tool_missing", "advisory.data_offline"},
	"JobState":           {"queued", "leased", "running", "finalizing", "passed", "failed", "cancelled", "lost", "retryable", "needs-operator"},
	"FailureClass":       {"code", "infra", "timeout", "cancelled", "lost"},
	"TrustClass":         {"owner", "collaborator", "internal", "untrusted"},
	"Isolation":          {"process", "container", "sandboxed-container", "vm", "ephemeral-vm", "hosted-disposable"},
	"NetworkScope":       {"none", "internet", "restricted", "lan", "privileged"},
	"SecretClass":        {"none", "project", "environment", "team", "release", "deploy"},
	"PrivacyZone":        {"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"},
	"SelectionMode":      {"full", "affected"},
	"CheckResult":        {"pass", "fail", "skip", "error"},
	"Result":             {"pass", "fail", "error", "cancelled"},
	"EventType":          {"queued", "planned", "leased", "started", "log", "check_result", "retrying", "cancelled", "needs_operator", "finished", "evidence"},
	"WorktreeState":      {"clean", "dirty"},
	"BindingState":       {"bound", "unbound"},
	"BindingReason":      {"dirty", "sha_mismatch"},
	"FreshnessState":     {"current", "stale", "unknown"},
	"ProducerKind":       {"coordinator-recorded", "signed-remote", "self-reported"},
	"SignatureKind":      {"coordinator-recorded", "runner-ed25519"},
	"Trigger":            {"push", "pull_request", "tag", "manual", "schedule", "api", "local"},
	"PipelineKind":       {"default", "named", "drill", "adhoc"},
	"JobKind":            {"static", "build", "test", "quality", "advanced", "release", "drill"},
	"Preset":             {"default", "strict"},
	"Priority":           {"low", "normal", "high"},
	"JobPath":            {"fast", "deep"},
	"ArtifactKind":       {"package", "binary", "archive", "image", "report", "coverage", "log", "benchmark", "sbom", "other"},
	"ArtifactVisibility": {"project", "restricted"},
}

// EnumValues returns a copy of the named v1 list.
func EnumValues(name string) []string { return append([]string(nil), enumValues[name]...) }
func validEnum(name, value string) bool {
	for _, item := range enumValues[name] {
		if item == value {
			return true
		}
	}
	return false
}
func (v Reason) Valid() bool             { return validEnum("Reason", string(v)) }
func (v JobState) Valid() bool           { return validEnum("JobState", string(v)) }
func (v FailureClass) Valid() bool       { return validEnum("FailureClass", string(v)) }
func (v TrustClass) Valid() bool         { return validEnum("TrustClass", string(v)) }
func (v Isolation) Valid() bool          { return validEnum("Isolation", string(v)) }
func (v NetworkScope) Valid() bool       { return validEnum("NetworkScope", string(v)) }
func (v SecretClass) Valid() bool        { return validEnum("SecretClass", string(v)) }
func (v PrivacyZone) Valid() bool        { return validEnum("PrivacyZone", string(v)) }
func (v SelectionMode) Valid() bool      { return validEnum("SelectionMode", string(v)) }
func (v CheckResult) Valid() bool        { return validEnum("CheckResult", string(v)) }
func (v Result) Valid() bool             { return validEnum("Result", string(v)) }
func (v EventType) Valid() bool          { return validEnum("EventType", string(v)) }
func (v WorktreeState) Valid() bool      { return validEnum("WorktreeState", string(v)) }
func (v BindingState) Valid() bool       { return validEnum("BindingState", string(v)) }
func (v BindingReason) Valid() bool      { return validEnum("BindingReason", string(v)) }
func (v FreshnessState) Valid() bool     { return validEnum("FreshnessState", string(v)) }
func (v ProducerKind) Valid() bool       { return validEnum("ProducerKind", string(v)) }
func (v SignatureKind) Valid() bool      { return validEnum("SignatureKind", string(v)) }
func (v Trigger) Valid() bool            { return validEnum("Trigger", string(v)) }
func (v PipelineKind) Valid() bool       { return validEnum("PipelineKind", string(v)) }
func (v JobKind) Valid() bool            { return validEnum("JobKind", string(v)) }
func (v Preset) Valid() bool             { return validEnum("Preset", string(v)) }
func (v Priority) Valid() bool           { return validEnum("Priority", string(v)) }
func (v JobPath) Valid() bool            { return validEnum("JobPath", string(v)) }
func (v ArtifactKind) Valid() bool       { return validEnum("ArtifactKind", string(v)) }
func (v ArtifactVisibility) Valid() bool { return validEnum("ArtifactVisibility", string(v)) }
