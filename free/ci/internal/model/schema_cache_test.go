package model

import "testing"

// D-0972: ValidateJSON re-parsed and re-resolved the schema on every call,
// which held the Verify fuzz target to about 150 execs/s.
func TestValidateJSONResolvesSchemaOnce(t *testing.T) {
	schemas, err := Schemas()
	if err != nil {
		t.Fatal(err)
	}
	schema := schemas["ci-events.v1.schema.json"]
	doc := []byte(`{}`)
	_ = ValidateJSON(schema, doc) // warm: the first call may resolve
	before := schemaResolves.Load()
	for i := 0; i < 50; i++ {
		_ = ValidateJSON(schema, doc)
	}
	if got := schemaResolves.Load() - before; got != 0 {
		t.Fatalf("schema resolved %d times across 50 repeat calls, want 0 (cached)", got)
	}
}

// A cached schema must still reject, and a different schema must not share an entry.
func TestValidateJSONCacheKeepsSchemasApart(t *testing.T) {
	schemas, err := Schemas()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := ValidateJSON(schemas["ci-events.v1.schema.json"], []byte(`{}`)); err == nil {
			t.Fatal("events schema accepted an empty object")
		}
		if err := ValidateJSON(schemas["ci-evidence.v1.schema.json"], []byte(`{}`)); err == nil {
			t.Fatal("evidence schema accepted an empty object")
		}
	}
	if err := ValidateJSON([]byte(`{`), []byte(`{}`)); err == nil {
		t.Fatal("malformed schema accepted")
	}
}
