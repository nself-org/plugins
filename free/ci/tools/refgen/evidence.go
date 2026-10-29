package main

// Purpose: the evidence field tables of Evidence-and-Machine-Output.md,
// built by reflection over model.Evidence so a new field shows up the next
// time the generator runs.
// Inputs:  the model.Evidence type; the closed value lists of model.
// Outputs: one Markdown table per struct, the root first, nested structs in
// first-use order.
// Constraints: a field is required unless its json tag has omitempty; a
// field of a closed-list type lists its allowed values.

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// evidenceTables renders the field tables of the evidence document.
func evidenceTables() string {
	var b strings.Builder
	queue := []reflect.Type{reflect.TypeOf(model.Evidence{})}
	seen := map[reflect.Type]bool{queue[0]: true}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		title := "`" + t.Name() + "`"
		if t.Name() == "Evidence" {
			title = "Evidence document (root)"
		}
		fmt.Fprintf(&b, "#### %s\n\n| Field | Type | Required | Values |\n|---|---|---|---|\n", title)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" || !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			required := "yes"
			if strings.Contains(opts, "omitempty") {
				required = "no"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", name, typeText(f.Type), required, valuesText(f.Type))
			if st := structOf(f.Type); st != nil && !seen[st] {
				seen[st] = true
				queue = append(queue, st)
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// structOf returns the model struct t holds directly or as an element, or nil.
func structOf(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && t.PkgPath() == modelPath {
		return t
	}
	return nil
}

// typeText is the JSON-facing type of t.
func typeText(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Pointer:
		return typeText(t.Elem()) + " or null"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "string (base64)"
		}
		return "list of " + typeText(t.Elem())
	case reflect.Map:
		return "map of " + typeText(t.Elem())
	case reflect.Struct:
		if t.PkgPath() == modelPath {
			return "`" + t.Name() + "`"
		}
		if t.String() == "time.Time" {
			return "timestamp"
		}
		return "object"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Interface:
		return "any"
	}
	return t.Kind().String()
}

// valuesText lists the closed values of a model string type, or "".
func valuesText(t reflect.Type) string {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.String || t.PkgPath() != modelPath {
		return ""
	}
	vals := model.EnumValues(t.Name())
	if len(vals) == 0 {
		return ""
	}
	return "`" + strings.Join(vals, "`, `") + "`"
}
