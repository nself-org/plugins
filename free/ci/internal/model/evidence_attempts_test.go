package model

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEvidenceAdditiveBackwardCompat: documents stored before the additive keys
// validate against the new schema, and the new keys validate and are bounded.
func TestEvidenceAdditiveBackwardCompat(t *testing.T) {
	schema, err := os.ReadFile(filepath.Join("..", "..", "schemas", "ci-evidence.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	old, err := filepath.Glob("testdata/valid/evidence.*.json")
	if err != nil {
		t.Fatal(err)
	}
	old = append(old, filepath.Join("..", "evidence", "testdata", "evidence.pre-auto21.json"))
	for _, path := range old {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateJSON(schema, body); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	bad, err := os.ReadFile("testdata/invalid/evidence.attempt-kind.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSON(schema, bad); err == nil {
		t.Error("an unknown runner kind must fail the schema")
	}
}
