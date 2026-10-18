package main

// Purpose: the placement-plan v1 schema (contract:ci.placement-plan,
// P7-SCHED-03), inferred from model.PlacementPlan with the constraints Go's
// type system cannot express. Output is deterministic.
// Constraints: the enum lists come from model.EnumValues; the optional
// `source` property is the explain wrapper's field (planview.Explanation).

import (
	"encoding/json"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/plugins/free/ci/internal/model"
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
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
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
	}
	if pl := job.Properties["placement"]; pl != nil {
		component(pl.Properties["components"])
		est := pl.Properties["estimate"]
		nonNegative(est.Properties["wait_ms"], est.Properties["runtime_ms"], est.Properties["cost_micro"])
		est.Properties["cost_confidence"] = conf("known", "computed", "estimated", "unknown")
	}
	component(alt.Properties["components"])
	alt.Properties["components"].Type, alt.Properties["components"].Types = "", []string{"null", "object"} // null when ineligible
	totals := s.Properties["totals"]
	nonNegative(totals.Properties["estimated_cost_micro"], totals.Properties["placed"], totals.Properties["waiting"], totals.Properties["awaiting_approval"], totals.Properties["unplaceable"])
	totals.Properties["cost_confidence"] = conf("known", "computed", "estimated", "unknown")
}
