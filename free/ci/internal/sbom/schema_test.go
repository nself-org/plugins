package sbom_test

import (
	"encoding/json"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func loadSchemaFile(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "schema", name))
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return &s
}

// cdxSchema resolves the vendored CycloneDX 1.6 schema; its two $refs are
// served from the vendored siblings, never from the network.
func cdxSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	loader := func(u *url.URL) (*jsonschema.Schema, error) {
		return loadSchemaFile(t, path.Base(u.Path)), nil
	}
	rs, err := loadSchemaFile(t, "bom-1.6.schema.json").Resolve(&jsonschema.ResolveOptions{Loader: loader})
	if err != nil {
		t.Fatalf("resolve CycloneDX 1.6 schema: %v", err)
	}
	return rs
}

// assertCycloneDXSchema fails the test unless doc validates against the vendored schema.
func assertCycloneDXSchema(t *testing.T, rs *jsonschema.Resolved, doc []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatal(err)
	}
	if err := rs.Validate(v); err != nil {
		t.Fatalf("document does not match the CycloneDX 1.6 schema: %v", err)
	}
}

func TestCycloneDXSchemaAcceptsGoldenAndRefusesBad(t *testing.T) {
	rs := cdxSchema(t)
	good, err := os.ReadFile(filepath.Join("testdata", "golden", "dir.cdx.json"))
	if err != nil {
		t.Fatal(err)
	}
	assertCycloneDXSchema(t, rs, good)
	var v map[string]any
	if err := json.Unmarshal(good, &v); err != nil {
		t.Fatal(err)
	}
	v["specVersion"] = 7 // wrong type
	if rs.Validate(v) == nil {
		t.Fatal("a document with a numeric specVersion passed the schema")
	}
	delete(v, "specVersion")
	delete(v, "bomFormat")
	if rs.Validate(v) == nil {
		t.Fatal("a document without bomFormat passed the schema")
	}
}
