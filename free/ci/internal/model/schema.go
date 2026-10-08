package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

// Schemas infers the public v1 contracts from the Go types and applies
// constraints that Go's type system cannot express.
func Schemas() (map[string][]byte, error) {
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{}}
	for name, values := range enumValues {
		var typ reflect.Type
		switch name {
		case "Reason":
			typ = reflect.TypeFor[Reason]()
		case "JobState":
			typ = reflect.TypeFor[JobState]()
		case "FailureClass":
			typ = reflect.TypeFor[FailureClass]()
		case "TrustClass":
			typ = reflect.TypeFor[TrustClass]()
		case "Isolation":
			typ = reflect.TypeFor[Isolation]()
		case "NetworkScope":
			typ = reflect.TypeFor[NetworkScope]()
		case "SecretClass":
			typ = reflect.TypeFor[SecretClass]()
		case "PrivacyZone":
			typ = reflect.TypeFor[PrivacyZone]()
		case "SelectionMode":
			typ = reflect.TypeFor[SelectionMode]()
		case "CheckResult":
			typ = reflect.TypeFor[CheckResult]()
		case "Result":
			typ = reflect.TypeFor[Result]()
		case "EventType":
			typ = reflect.TypeFor[EventType]()
		case "WorktreeState":
			typ = reflect.TypeFor[WorktreeState]()
		case "BindingState":
			typ = reflect.TypeFor[BindingState]()
		case "BindingReason":
			typ = reflect.TypeFor[BindingReason]()
		case "FreshnessState":
			typ = reflect.TypeFor[FreshnessState]()
		case "ProducerKind":
			typ = reflect.TypeFor[ProducerKind]()
		case "SignatureKind":
			typ = reflect.TypeFor[SignatureKind]()
		case "Trigger":
			typ = reflect.TypeFor[Trigger]()
		case "PipelineKind":
			typ = reflect.TypeFor[PipelineKind]()
		case "JobKind":
			typ = reflect.TypeFor[JobKind]()
		case "Preset":
			typ = reflect.TypeFor[Preset]()
		case "Priority":
			typ = reflect.TypeFor[Priority]()
		case "JobPath":
			typ = reflect.TypeFor[JobPath]()
		case "ArtifactKind":
			typ = reflect.TypeFor[ArtifactKind]()
		case "ArtifactVisibility":
			typ = reflect.TypeFor[ArtifactVisibility]()
		}
		if typ != nil {
			choices := make([]any, len(values))
			for i, v := range values {
				choices[i] = v
			}
			opts.TypeSchemas[typ] = &jsonschema.Schema{Type: "string", Enum: choices}
		}
	}
	config, err := jsonschema.For[PipelineConfig](opts)
	if err != nil {
		return nil, err
	}
	evidence, err := jsonschema.For[Evidence](opts)
	if err != nil {
		return nil, err
	}
	events, err := jsonschema.For[Event](opts)
	if err != nil {
		return nil, err
	}
	decorateConfig(config)
	decorateEvidence(evidence)
	decorateEvents(events)
	out := map[string][]byte{}
	for name, schema := range map[string]*jsonschema.Schema{"ci-pipeline-config.v1.schema.json": config, "ci-evidence.v1.schema.json": evidence, "ci-events.v1.schema.json": events} {
		b, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return nil, err
		}
		out[name] = append(b, '\n')
	}
	return out, nil
}

func decorateConfig(s *jsonschema.Schema) {
	s.Schema = "https://json-schema.org/draft/2020-12/schema"
	s.ID = "https://nself.org/schemas/ci-pipeline-config.v1.schema.json"
	s.Properties["version"].Const = anyPtr(1)
	s.Properties["support"].Items.Pattern = `^[a-z0-9]+/[a-z0-9_]+$`
	s.Properties["presets"].Items.Enum = choices("default", "strict")
	checks := s.Properties["checks"].AdditionalProperties
	checks.Properties["coverage_floor"].Minimum = floatPtr(0)
	checks.Properties["coverage_floor"].Maximum = floatPtr(100)
	job := s.Properties["jobs"].AdditionalProperties
	job.Properties["kind"].Enum = choices("static", "build", "test", "quality", "advanced", "release", "drill")
	job.Properties["run"].MinItems = intPtr(1)
	job.Properties["timeout"].Pattern = `^[+-]?(0|([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+)$`
	job.Properties["priority"].Enum = choices("low", "normal", "high")
	job.Properties["path"].Enum = choices("fast", "deep")
	job.Properties["isolation"].Enum = choices("process", "container")
	job.Properties["retry"].Properties["infra_max"].Minimum = floatPtr(0)
	job.Properties["retry"].Properties["infra_max"].Maximum = floatPtr(3)
	job.Properties["matrix"].Properties["platform"] = &jsonschema.Schema{OneOf: []*jsonschema.Schema{{Const: anyPtr("support")}, {Type: "array", Items: &jsonschema.Schema{Type: "string", Pattern: `^[a-z0-9]+/[a-z0-9_]+$`}}}}
	job.Properties["secrets"].Items.Pattern = `^(none|project|environment|team|release|deploy)/[A-Za-z_][A-Za-z0-9_.-]*(/[A-Za-z_][A-Za-z0-9_.-]*)?$`
	artifact := job.Properties["artifacts"].Items
	artifact.Properties["kind"].Enum = choices("package", "binary", "archive", "image", "report", "coverage", "log", "benchmark", "sbom", "other")
	artifact.Properties["visibility"].Enum = choices("project", "restricted")
	artifact.Properties["retention"].Pattern = `^(keep|[1-9][0-9]*(s|m|h|d))$`
	artifact.AllOf = []*jsonschema.Schema{{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"image": {}}, Required: []string{"image"}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"kind": {Const: anyPtr("image")}}, Required: []string{"kind"}}}}
	req := job.Properties["requires"]
	req.Properties["labels"].Items.Pattern = `^[a-z][a-z0-9_.-]{0,62}(=[A-Za-z0-9_.-]{0,63})?$`
	req.Properties["labels"].MaxItems = intPtr(16)
	req.Properties["tools"].MaxItems = intPtr(16)
	req.Properties["accelerators"].Items.Enum = choices("gpu")
	req.Properties["cpu"].Minimum = floatPtr(1)
	req.Properties["mem_mb"].Minimum = floatPtr(1)
	s.Properties["pipelines"].AdditionalProperties.Properties["triggers"].Items.Enum = choices("push", "pull_request", "tag", "manual", "schedule", "api")
	s.Properties["pipelines"].AdditionalProperties.Properties["max_age"].Pattern = job.Properties["timeout"].Pattern
	// Policy is intentionally opaque: the protected policy loader owns it.
}

func decorateEvidence(s *jsonschema.Schema) {
	s.Schema = "https://json-schema.org/draft/2020-12/schema"
	s.ID = "https://nself.org/schemas/ci-evidence.v1.schema.json"
	s.Properties["schema"].Const = anyPtr("ci.evidence/v1")
	s.Properties["revision"].Pattern = `^([0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`
	s.Properties["worktree_state"].Enum = choices("clean", "dirty")
	s.Properties["trigger"].Enum = choices("push", "pull_request", "tag", "manual", "schedule", "api", "local")
	s.Properties["pipeline"].Properties["kind"].Enum = choices("default", "named", "drill", "adhoc")
	s.Properties["binding"].Properties["state"].Enum = choices("bound", "unbound")
	s.Properties["binding"].Properties["reason"].Enum = choices(nil, "dirty", "sha_mismatch")
	s.Properties["producer"].Properties["kind"].Enum = choices("coordinator-recorded", "signed-remote", "self-reported")
	s.Properties["signature"].Properties["kind"].Enum = choices("coordinator-recorded", "runner-ed25519")
	s.Properties["freshness"].Properties["state"].Enum = choices("current", "stale", "unknown")
	s.Properties["selection"].Properties["mode"].Enum = choices("full", "affected")
	s.Properties["checks"].Items.Properties["excerpt"].MaxLength = intPtr(4096)
	allowedFailure := &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"allow_failure": {Const: anyPtr(true)}}, Required: []string{"allow_failure"}}
	badCheck := &jsonschema.Schema{
		Properties: map[string]*jsonschema.Schema{"required": {Const: anyPtr(true)}},
		Required:   []string{"required"},
		AnyOf: []*jsonschema.Schema{
			{Properties: map[string]*jsonschema.Schema{"result": {Enum: choices("skip", "error")}}, Required: []string{"result"}},
			{Properties: map[string]*jsonschema.Schema{"result": {Const: anyPtr("fail")}}, Required: []string{"result"}, Not: allowedFailure},
		},
	}
	// A dirty tree is never bound; an unbound document is never current.
	s.AllOf = []*jsonschema.Schema{
		{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"worktree_state": {Const: anyPtr("dirty")}}, Required: []string{"worktree_state"}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"binding": {Properties: map[string]*jsonschema.Schema{"state": {Const: anyPtr("unbound")}}}}}},
		{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"binding": {Properties: map[string]*jsonschema.Schema{"state": {Const: anyPtr("unbound")}}, Required: []string{"state"}}}, Required: []string{"binding"}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"freshness": {Properties: map[string]*jsonschema.Schema{"state": {Not: &jsonschema.Schema{Const: anyPtr("current")}}}}}}},
		{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"worktree_state": {Const: anyPtr("clean")}}, Required: []string{"worktree_state"}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"tracked_changes_digest": {Const: anyPtr(nil)}}}},
		{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"result": {Const: anyPtr("pass")}}, Required: []string{"result"}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"checks": {Contains: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"substantive": {Const: anyPtr(true)}, "result": {Enum: choices("pass", "fail")}}, Required: []string{"substantive", "result"}}}}}},
		{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"checks": {Contains: badCheck}}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"result": {Not: &jsonschema.Schema{Const: anyPtr("pass")}}}}},
	}
}

func decorateEvents(s *jsonschema.Schema) {
	s.Schema = "https://json-schema.org/draft/2020-12/schema"
	s.ID = "https://nself.org/schemas/ci-events.v1.schema.json"
	s.Properties["schema"].Const = anyPtr("ci.events/v1")
	s.Properties["seq"].Minimum = floatPtr(1)
}

func choices(items ...any) []any  { return items }
func anyPtr(v any) *any           { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

// ValidateJSON returns a JSON-pointer-bearing error for an invalid document.
func ValidateJSON(schemaBytes, document []byte) error {
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return err
	}
	r, err := schema.Resolve(nil)
	if err != nil {
		return err
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(document))
	if err := dec.Decode(&value); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("schema validation at /: trailing JSON value")
		}
		return fmt.Errorf("schema validation at /: trailing JSON: %w", err)
	}
	if err := r.Validate(value); err != nil {
		return fmt.Errorf("schema validation at %s", err)
	}
	if schema.ID == "https://nself.org/schemas/ci-evidence.v1.schema.json" {
		if doc, ok := value.(map[string]any); ok {
			if checks, ok := doc["checks"].([]any); ok {
				for i, raw := range checks {
					if check, ok := raw.(map[string]any); ok {
						if excerpt, ok := check["excerpt"].(string); ok && len(excerpt) > 4096 {
							return fmt.Errorf("schema validation at /checks/%d/excerpt: exceeds 4096 UTF-8 bytes", i)
						}
					}
				}
			}
		}
	}
	return nil
}
