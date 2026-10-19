package main

// Purpose: the release-manifest v1 schema (contract:ci.release-manifest,
// P7-CACHE-05), generated from manifest.Manifest in internal/release/manifest.
// Inputs:  none. Outputs: schemas/release-manifest.v1.schema.json, relative to
// the working directory exactly like main.go (tools/schemagen or internal/model,
// both two levels below the module root).
// Constraints: main.go keeps no registry yet, so this file registers itself
// from init() and writes its own schema before main runs; a registry in main.go
// would replace this (shared file, not edited here). The `go:generate` line
// makes `go generate ./tools/schemagen/...` regenerate every schema.

//go:generate go run .

import (
	"log"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

func init() {
	b, err := manifest.JSONSchema()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("..", "..", "schemas", manifest.SchemaFile), b, 0644); err != nil {
		log.Fatal(err)
	}
}
