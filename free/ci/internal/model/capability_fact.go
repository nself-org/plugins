package model

import (
	"encoding/json"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

// Fact records a value and the evidence for it. Unknown values are nil.
type Fact[T any] struct {
	Value      *T        `json:"value"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	Confidence string    `json:"confidence"`
}

func (f Fact[T]) Valid() bool {
	if f.Source != "self" && f.Source != "probe" && f.Source != "provider" && f.Source != "assigned" {
		return false
	}
	if f.ObservedAt.IsZero() {
		return false
	}
	if f.Confidence != "known" && f.Confidence != "estimated" && f.Confidence != "unknown" {
		return false
	}
	return (f.Value == nil) == (f.Confidence == "unknown")
}

// CapabilitySchema is generated from the Go contract with fact invariants attached.
func CapabilitySchema() ([]byte, error) {
	s, err := jsonschema.For[Capability](nil)
	if err != nil {
		return nil, err
	}
	s.Schema = "https://json-schema.org/draft/2020-12/schema"
	s.ID = "https://nself.org/schemas/runner-capability.v1.schema.json"
	s.Properties["schema"].Const = anyPtr("ci.runner-capability/v1")
	decorateCapability(s)
	identity := s.Properties["identity"]
	identity.Properties["provider"].Enum = choices("local", "ssh", "agent", "github-hosted", "github-jit", "gitlab", "ephemeral")
	identity.Properties["transport"].Enum = choices("local", "ssh", "agent", "provider")
	identity.Properties["ownership"].Properties["value"].Enum = choices(nil, "operator", "team", "provider")
	s.Properties["platform"].Properties["virtualization"].Properties["value"].Enum = choices(nil, "none", "container", "vm", "unknown")
	s.Properties["availability"].Properties["state"].Properties["value"].Enum = choices(nil, "online", "offline", "draining", "maintenance", "revoked")
	s.Properties["availability"].Properties["interactive"].Properties["value"].Enum = choices(nil, "active", "idle", "unknown")
	s.Properties["location"].Properties["kind"].Properties["value"].Enum = choices(nil, "local", "lan", "remote", "hosted")
	s.Properties["economics"].Properties["class"].Properties["value"].Enum = choices(nil, "local", "owned", "included", "metered", "ephemeral")
	trust := s.Properties["trust"]
	trust.Properties["accepts"].Properties["value"].Items.Enum = stringChoices(EnumValues("TrustClass"))
	trust.Properties["isolation"].Properties["value"].Enum = append([]any{nil}, stringChoices(EnumValues("Isolation"))...)
	trust.Properties["network"].Properties["value"].Items.Enum = stringChoices(EnumValues("NetworkScope"))
	trust.Properties["secret_classes"].Properties["value"].Items.Enum = stringChoices(EnumValues("SecretClass"))
	trust.Properties["privacy_zone"].Properties["value"].Enum = append([]any{nil}, stringChoices(EnumValues("PrivacyZone"))...)
	for key, field := range trust.Properties {
		field.Properties["source"].Enum = choices("assigned")
		if key == "accepts" {
			field.Properties["source"].Enum = choices("assigned", "provider")
			field.AllOf = append(field.AllOf, &jsonschema.Schema{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"value": {Const: anyPtr(nil)}}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"source": {Const: anyPtr("provider")}}}})
		}
	}
	s.Properties["verified"].Properties["by"].Enum = choices("self", "probe", "provider")
	s.Properties["deploy_host"].Properties["source"].Enum = choices(nil, "inventory", "never-list", "marker")
	b, err := json.MarshalIndent(s, "", "  ")
	return append(b, '\n'), err
}

func stringChoices(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func decorateCapability(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if s.Properties != nil && s.Properties["value"] != nil && s.Properties["confidence"] != nil && s.Properties["source"] != nil {
		s.Required = []string{"value", "source", "observed_at", "confidence"}
		s.Properties["source"].Enum = choices("self", "probe", "provider", "assigned")
		s.Properties["confidence"].Enum = choices("known", "estimated", "unknown")
		s.Properties["observed_at"].MinLength = intPtr(1)
		s.AllOf = append(s.AllOf,
			&jsonschema.Schema{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"confidence": {Const: anyPtr("unknown")}}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"value": {Const: anyPtr(nil)}}}},
			&jsonschema.Schema{If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"confidence": {Enum: choices("known", "estimated")}}}, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"value": {Not: &jsonschema.Schema{Const: anyPtr(nil)}}}}},
		)
	}
	for _, p := range s.Properties {
		decorateCapability(p)
	}
	for _, p := range s.Defs {
		decorateCapability(p)
	}
	decorateCapability(s.Items)
	decorateCapability(s.AdditionalProperties)
}
