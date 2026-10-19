package main

// Purpose: writes the deterministic v1 contract schemas from the domain structs.
// Inputs:  none. Outputs: every registry entry as a file in schemas/, relative
// to the working directory (tools/schemagen or internal/model, both two levels
// below the module root).
// Constraints: a new schema is one registry() entry; nothing writes from init().

//go:generate go run .

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

// generator returns the bytes of one schema file.
type generator func() ([]byte, error)

// modelSchema picks one file out of model.Schemas, which infers several.
func modelSchema(name string) generator {
	return func() ([]byte, error) {
		all, err := model.Schemas()
		if err != nil {
			return nil, err
		}
		b, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("model.Schemas has no %s", name)
		}
		return b, nil
	}
}

// registry maps each schema file name to the function that generates it.
func registry() map[string]generator {
	return map[string]generator{
		"ci-events.v1.schema.json":          modelSchema("ci-events.v1.schema.json"),
		"ci-evidence.v1.schema.json":        modelSchema("ci-evidence.v1.schema.json"),
		"ci-pipeline-config.v1.schema.json": modelSchema("ci-pipeline-config.v1.schema.json"),
		"runner-capability.v1.schema.json":  model.CapabilitySchema,
		"runner-protocol.v1.schema.json":    protocol.Schema,
		"placement-plan.v1.schema.json":     placementSchema,
		manifest.SchemaFile:                 manifest.JSONSchema,
	}
}

// generate writes every registered schema into dir.
func generate(dir string) error {
	for name, gen := range registry() {
		data, err := gen()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := generate(filepath.Join("..", "..", "schemas")); err != nil {
		log.Fatal(err)
	}
}
