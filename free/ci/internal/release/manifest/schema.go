package manifest

// Purpose: the generated JSON Schema of contract:ci.release-manifest v1
// (schemas/release-manifest.v1.schema.json), inferred from Manifest with the
// rules Go's types cannot express. tools/schemagen writes it; a drift test here
// regenerates it and compares.
// Inputs:  the Manifest type. Outputs: deterministic indented schema bytes.
// Constraints: unknown additive fields stay allowed (X19), so no object is
// closed; the schema const pins the v1 major, so a v2 payload fails it.

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

// SchemaFile is the file name of the generated schema.
const SchemaFile = "release-manifest.v1.schema.json"

// JSONSchema returns the schema of Manifest.
func JSONSchema() ([]byte, error) {
	s, err := jsonschema.For[Manifest](nil)
	if err != nil {
		return nil, err
	}
	decorate(s)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func pattern(re string) *jsonschema.Schema { return &jsonschema.Schema{Type: "string", Pattern: re} }

func constant(v any) *jsonschema.Schema { return &jsonschema.Schema{Const: &v} }

// open lets an object carry fields a later minor adds, and makes nullable
// lists real arrays (the document never carries a null list).
func open(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) == 2 && s.Types[0] == "null" && s.Types[1] == "array" {
		s.Type, s.Types = "array", nil
	}
	if s.Type == "object" || len(s.Properties) > 0 {
		s.AdditionalProperties = nil
	}
	for _, p := range s.Properties {
		open(p)
	}
	open(s.Items)
}

func decorate(s *jsonschema.Schema) {
	s.Schema = "https://json-schema.org/draft/2020-12/schema"
	s.ID = "https://nself.org/schemas/" + SchemaFile
	open(s)
	one := 1
	p := s.Properties
	p["schema"].Const = anyPtr(Schema)
	p["repo_id"].MinLength = &one
	p["revision"] = pattern(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	rel := p["release"].Properties
	for _, k := range []string{"run_id", "pipeline", "job_id", "attempt_id", "idempotency_key"} {
		rel[k].MinLength = &one
	}
	rel["finished_at"].Format = "date-time"
	ev := p["evidence"].Properties
	ev["path"], ev["result"], ev["class"] = constant(EvidencePath), constant(EvidenceResult), constant(EvidenceClass)
	ev["sha256"] = pattern(`^[0-9a-f]{64}$`)
	p["artifacts"].MinItems = &one
	art := p["artifacts"].Items.Properties
	art["name"] = pattern(`^[A-Za-z0-9._-][A-Za-z0-9._/-]{0,199}$`)
	art["kind"].Enum = []any{KindPackage, KindBinary, KindArchive, KindImage, KindReport, KindOther}
	art["sha256"] = pattern(`^[0-9a-f]{64}$`)
	art["platform"] = &jsonschema.Schema{Types: []string{"null", "string"}, Pattern: `^[a-z0-9]+/[a-z0-9]+$`}
	zero := 0.0
	art["size"].Minimum = &zero
	if im := art["image"]; im != nil {
		im.Types, im.Type = []string{"null", "object"}, ""
		im.Properties["index_digest"] = pattern(`^sha256:[0-9a-f]{64}$`)
		im.Properties["manifests"].MinItems = &one
		im.Properties["manifests"].Items.Properties["digest"] = pattern(`^sha256:[0-9a-f]{64}$`)
	}
	for _, k := range []string{"cyclonedx", "spdx"} {
		art["sbom"].Properties[k].Properties["sha256"] = pattern(`^[0-9a-f]{64}$`)
	}
	prov := p["provenance"].Properties
	prov["path"], prov["predicate_type"], prov["slsa_build_level"] = constant(ProvenancePath), constant(ProvenancePredicate), constant(SLSALevel)
	prov["sha256"] = pattern(`^[0-9a-f]{64}$`)
	p["checksums"].Properties["path"] = constant(ChecksumsPath)
	p["checksums"].Properties["sha256"] = pattern(`^[0-9a-f]{64}$`)
	p["signing"].Properties["purpose"] = constant(SigningPurpose)
	p["signing"].Properties["key_id"].MinLength = &one
}

func anyPtr(v any) *any { return &v }
