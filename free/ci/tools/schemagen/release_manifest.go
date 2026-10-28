package main

// Purpose: the release-manifest v1 schema (contract:ci.release-manifest,
// P7-CACHE-05), generated from manifest.Manifest in internal/release/manifest.
// Inputs:  none. Outputs: schemas/release-manifest.v1.schema.json, written by main.go
// from the registry, relative to the working directory (tools/schemagen or internal/model,
// both two levels below the module root).

import "github.com/nself-org/plugins/free/ci/internal/release/manifest"

func init() { register(manifest.SchemaFile, manifest.JSONSchema) }
