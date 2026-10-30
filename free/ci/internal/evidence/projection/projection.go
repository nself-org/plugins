// Package projection is the pure function behind `nself-ci/1`: the flat view of
// a ci.evidence/v1 document that external parsers read. It is not a second
// schema (AUTO D8): every projected field names the evidence key it comes from,
// and a key an older document lacks projects as null with a reason.
package projection

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Name is the one projection this package knows.
const Name = "nself-ci/1"

// Value is one projected field: the value (nil when the source key is absent)
// and, when nil, why.
type Value struct {
	Value  any    `json:"value"`
	Reason string `json:"reason,omitempty"`
}

// Projected is the nself-ci/1 view. Fields marshals with sorted keys.
type Projected struct {
	Projection string           `json:"projection"`
	Fields     map[string]Value `json:"fields"`
}

// Mapping names a projected field and the evidence key path it reads.
type Mapping struct {
	Field  string
	Source string // dotted path; "[]" marks a per-item list
}

// scalars are read by path from the document, in page order.
var scalars = []Mapping{
	{"revision", "revision"},
	{"run_id", "run_id"},
	{"result", "result"},
	{"source_trust", "source_trust"},
	{"binding", "binding.state"},
	{"freshness", "freshness.state"},
	{"runner_id", "runner.id"},
	{"runner_isolation", "runner.isolation"},
	{"duration_ms", "timings.duration_ms"},
	{"checks_total", "counts.total"},
	{"checks_pass", "counts.pass"},
	{"checks_fail", "counts.fail"},
	{"checks_skip", "counts.skip"},
	{"checks_error", "counts.error"},
}

// listFields are lists whose items are projected key by key.
var listFields = []struct {
	Field string
	Root  string
	Items []Mapping // Source is relative to one item
}{
	{"attempts", "attempts", []Mapping{
		{"job_id", "job_id"}, {"attempt", "attempt"},
		{"runner_kind", "runner.kind"}, {"runner_tier", "runner.tier"}, {"runner_pool", "runner.pool"}, {"runner_rank", "runner.rank"},
		{"queue_ms", "timings.queue_ms"}, {"transfer_ms", "timings.transfer_ms"}, {"run_ms", "timings.run_ms"},
	}},
	{"errors", "errors", []Mapping{{"code", "code"}, {"job_id", "job_id"}, {"attempt", "attempt"}, {"detail", "detail"}}},
}

// flakySource is the one derived field: the ids of checks carrying `flaky`.
const flakySource = "checks[].flaky"

// Mappings lists every projected field with its source key, for the generated page.
func Mappings() []Mapping {
	out := append([]Mapping(nil), scalars...)
	for _, l := range listFields {
		out = append(out, Mapping{l.Field, l.Root + "[]"})
		for _, m := range l.Items {
			out = append(out, Mapping{l.Field + "[]." + m.Field, l.Root + "[]." + m.Source})
		}
	}
	return append(out, Mapping{"flaky_checks", flakySource})
}

// Project reads one ci.evidence/v1 document (any age) and returns its view.
func Project(doc []byte, name string) (Projected, error) {
	if name != Name {
		return Projected{}, fmt.Errorf("projection: unknown projection %q", name)
	}
	var root map[string]any
	if err := json.Unmarshal(doc, &root); err != nil {
		return Projected{}, fmt.Errorf("projection: not a JSON object: %w", err)
	}
	out := Projected{Projection: Name, Fields: map[string]Value{}}
	for _, m := range scalars {
		out.Fields[m.Field] = lookup(root, m.Source)
	}
	for _, l := range listFields {
		items, ok := root[l.Root].([]any)
		if !ok {
			out.Fields[l.Field] = missing(l.Root)
			continue
		}
		rows := make([]map[string]Value, 0, len(items))
		for _, it := range items {
			obj, _ := it.(map[string]any)
			row := map[string]Value{}
			for _, m := range l.Items {
				row[m.Field] = lookup(obj, m.Source)
			}
			rows = append(rows, row)
		}
		out.Fields[l.Field] = Value{Value: rows}
	}
	out.Fields["flaky_checks"] = flaky(root)
	return out, nil
}

func missing(path string) Value {
	return Value{Reason: "source key absent: " + path}
}

// lookup follows a dotted path; a missing step or a JSON null is nil with a reason.
func lookup(obj map[string]any, path string) Value {
	var cur any = obj
	for _, step := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return missing(path)
		}
		if cur, ok = m[step]; !ok {
			return missing(path)
		}
	}
	if cur == nil {
		return Value{Reason: "source key is null: " + path}
	}
	return Value{Value: cur}
}

func flaky(root map[string]any) Value {
	checks, ok := root["checks"].([]any)
	if !ok {
		return missing("checks")
	}
	ids := []string{}
	for _, c := range checks {
		m, _ := c.(map[string]any)
		if m["flaky"] != nil {
			id, _ := m["id"].(string)
			ids = append(ids, id)
		}
	}
	return Value{Value: ids}
}
