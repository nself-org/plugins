package main

// Purpose: the placement-plan v1 schema (contract:ci.placement-plan,
// P7-SCHED-03), inferred from model.PlacementPlan with the constraints Go's
// type system cannot express. Output is deterministic.
// Constraints: the enum lists come from model.EnumValues; the optional
// `source` property is the explain wrapper's field (planview.Explanation).

import (
	"encoding/json"
	"reflect"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func enumOf(name string, extra ...any) []any {
	var out []any
	for _, v := range model.EnumValues(name) {
		out = append(out, v)
	}
	return append(out, extra...)
}

func placementSchema() ([]byte, error) {
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[model.Isolation]():    {Type: "string", Enum: enumOf("Isolation", "")},
		reflect.TypeFor[model.NetworkScope](): {Type: "string", Enum: enumOf("NetworkScope", "")},
		reflect.TypeFor[model.SecretClass]():  {Type: "string", Enum: enumOf("SecretClass")},
	}}
	s, err := jsonschema.For[model.PlacementPlan](opts)
	if err != nil {
		return nil, err
	}
	decoratePlacement(s)
	nullFreeArrays(s)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// nullFreeArrays makes every array of the contract a real array: the
// inference allows null for a nil slice, but the document never carries one.
func nullFreeArrays(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) == 2 && s.Types[0] == "null" && s.Types[1] == "array" {
		s.Type, s.Types = "array", nil
	}
	for _, p := range s.Properties {
		nullFreeArrays(p)
	}
	nullFreeArrays(s.Items)
	nullFreeArrays(s.AdditionalProperties)
}

func presetNames() []any {
	var names []string
	for n := range sched.Presets() {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, n := range names {
		out[i] = n
	}
	return out
}

func nonNegative(s ...*jsonschema.Schema) {
	zero := 0.0
	for _, x := range s {
		x.Minimum = &zero
	}
}

func decoratePlacement(s *jsonschema.Schema) {
	one := 1
	conf := func(values ...any) *jsonschema.Schema { return &jsonschema.Schema{Type: "string", Enum: values} }
	s.Schema = "https://json-schema.org/draft/2020-12/schema"
	s.ID = "https://nself.org/schemas/placement-plan.v1.schema.json"
	c := any("ci.placement-plan/v1")
	s.Properties["schema"].Const = &c
	// The explain output adds where the facts came from; a plan omits it.
	s.Properties["source"] = &jsonschema.Schema{Type: "string", Enum: []any{"plan", "decision-record"}}
	pipe := s.Properties["pipeline"]
	pipe.Properties["world_digest"].MinLength = &one
	pipe.Properties["policy_digest"].MinLength = &one
	pipe.Properties["trigger"].Enum = enumOf("Trigger", "")
	pipe.Properties["preset"].Enum = presetNames()
	s.Properties["fast_path"].Properties["reason"].Enum = []any{"none", "no_remote", "unknown_estimates", "overhead_exceeds_gain"}
	job := s.Properties["jobs"].Items
	job.Properties["band"].Enum = []any{"interactive", "normal", "background"}
	job.Properties["status"].Enum = []any{"placed", "waiting", "awaiting-approval", "unplaceable"}
	req := job.Properties["requirements"]
	nonNegative(req.Properties["cpu"], req.Properties["mem_mb"])
	job.Properties["reasons"].Items.Properties["code"].MinLength = &one
	alt := job.Properties["alternatives"].Items
	alt.Properties["reasons"].Items.Properties["code"].MinLength = &one
	component := func(m *jsonschema.Schema) {
		if m == nil || m.AdditionalProperties == nil {
			return
		}
		v := m.AdditionalProperties
		nonNegative(v.Properties["weight_permille"])
		v.Properties["confidence"] = conf("known", "estimated", "unknown")
		m.PropertyNames = &jsonschema.Schema{Enum: []any{"wait", "runtime", "cold_start", "transfer", "cache", "contention", "cost", "scarcity", "reliability"}}
	}
	if pl := job.Properties["placement"]; pl != nil {
		pl.Properties["location"].Enum = []any{"local", "lan", "remote", "hosted"}
		pl.Properties["cost_class"].Enum = []any{"local", "owned", "included", "metered", "ephemeral"}
		component(pl.Properties["components"])
		pl.Properties["components"].Type, pl.Properties["components"].Types = "object", nil
		est := pl.Properties["estimate"]
		nonNegative(est.Properties["wait_ms"], est.Properties["runtime_ms"], est.Properties["cost_micro"])
		est.Properties["cost_confidence"] = conf("known", "computed", "estimated", "unknown")
	}
	component(alt.Properties["components"])
	alt.Properties["components"].Type, alt.Properties["components"].Types = "", []string{"null", "object"} // null when ineligible
	// A placed job has a placement and any other job has none.
	placedIf := &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"status": {Const: anyOf("placed")}}, Required: []string{"status"}}
	job.AllOf = []*jsonschema.Schema{
		{If: placedIf, Then: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"placement": {Type: "object"}}}, Else: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"placement": {Type: "null"}}}},
	}
	totals := s.Properties["totals"]
	nonNegative(totals.Properties["estimated_cost_micro"], totals.Properties["placed"], totals.Properties["waiting"], totals.Properties["awaiting_approval"], totals.Properties["unplaceable"])
	desc := totals.Properties["cost_confidence"].Description
	totals.Properties["cost_confidence"] = conf("known", "computed", "estimated", "unknown")
	totals.Properties["cost_confidence"].Description = desc
}

func anyOf(v any) *any { return &v }
