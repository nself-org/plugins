package inventory

// Purpose: keeps schemas/inventory.v1.schema.json and the Entry struct in step (P7-AUTO-04).
// Inputs:  the schema file and the yaml tags of Entry and Document.
// Outputs: pass or fail.
// Constraints: file read only.

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

func tagNames(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		out = append(out, strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0])
	}
	sort.Strings(out)
	return out
}

func keysOf(m map[string]json.RawMessage) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestSchemaMatchesEntry(t *testing.T) {
	raw, err := os.ReadFile("../../../schemas/inventory.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err = json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keysOf(top.Properties), tagNames(reflect.TypeOf(Document{}))) {
		t.Fatalf("document fields: schema %v struct %v", keysOf(top.Properties), tagNames(reflect.TypeOf(Document{})))
	}
	var entries struct {
		Items struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"items"`
	}
	if err = json.Unmarshal(top.Properties["entries"], &entries); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keysOf(entries.Items.Properties), tagNames(reflect.TypeOf(Entry{}))) {
		t.Fatalf("entry fields: schema %v struct %v", keysOf(entries.Items.Properties), tagNames(reflect.TypeOf(Entry{})))
	}
	if slices.Contains(keysOf(entries.Items.Properties), "slots") {
		t.Fatal("slots are agent-side")
	}
}
