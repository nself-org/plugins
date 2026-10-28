package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

// TestRegistryOwnsPerTypeSchemas (D-1079): placement and release-manifest are registered, and the
// bytes the registry yields are the committed files (so `go generate ./tools/schemagen/...` is a no-op).
func TestRegistryOwnsPerTypeSchemas(t *testing.T) {
	got, err := registered()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"placement-plan.v1.schema.json", manifest.SchemaFile} {
		b, ok := got[name]
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		disk, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, disk) {
			t.Errorf("%s is stale: run `go generate ./tools/schemagen/...`", name)
		}
	}
}

// TestRegisterRejectsADuplicate: two types may not share a schema file.
func TestRegisterRejectsADuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a second register of the same name did not panic")
		}
	}()
	register(manifest.SchemaFile, manifest.JSONSchema)
}
