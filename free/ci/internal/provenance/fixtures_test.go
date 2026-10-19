package provenance_test

// Purpose: shared fixtures for the provenance tests: one release and one agent
// build whose facts every rule accepts, and the vendored schemas.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

func hex64(c string) string { return strings.Repeat(c, 64) }

// releaseFacts returns facts for a public fixture repository; its lists are
// deliberately out of order so Build has to sort them.
func releaseFacts() provenance.ReleaseFacts {
	return provenance.ReleaseFacts{
		Visibility: attest.Public, RepoID: "local:" + hex64("e"), Revision: strings.Repeat("a", 40),
		Pipeline: "release", Job: "publish", ConfigDigest: hex64("c"), Isolation: "container", NselfCIVersion: "1.5.0",
		Lockfiles:   []provenance.FileDigest{{Name: "pnpm-lock.yaml", SHA256: hex64("2")}, {Name: "go.sum", SHA256: hex64("1")}},
		Image:       &provenance.ImageDigest{Ref: "ghcr.io/example/toolchain:1", SHA256: hex64("3")},
		Coordinator: "ms5.camarata.lan", Runner: "runner-7",
		RunID: "run-1", AttemptID: "run-1.publish-1a2b3c4d.a1",
		StartedOn: time.Date(2026, 10, 19, 11, 0, 5, 700, time.UTC), FinishedOn: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		Subjects: []provenance.FileDigest{{Name: "nself-linux-amd64", SHA256: hex64("5")}, {Name: "image/nself", SHA256: hex64("d")}},
		Evidence: hex64("6"),
		SBOMs: []provenance.FileDigest{{Name: "sbom/nself-linux-amd64.spdx.json", SHA256: hex64("8")}, {Name: "sbom/image/nself.cdx.json", SHA256: hex64("9")},
			{Name: "sbom/nself-linux-amd64.cdx.json", SHA256: hex64("7")}, {Name: "sbom/image/nself.spdx.json", SHA256: hex64("0")}},
	}
}

func agentFacts() provenance.AgentFacts {
	return provenance.AgentFacts{
		Repo: "nself-org/plugins", Ref: "refs/tags/agent-v1.5.0", Revision: strings.Repeat("b", 40), RunID: "1234567890",
		StartedOn: time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC), FinishedOn: time.Date(2026, 10, 19, 11, 30, 0, 0, time.UTC),
		Subjects: []provenance.FileDigest{{Name: "nself-ci-agent-linux-arm64", SHA256: hex64("2")}, {Name: "nself-ci-agent-linux-amd64", SHA256: hex64("1")}},
		SBOMs:    []provenance.FileDigest{{Name: "nself-ci-agent-linux-amd64.cdx.json", SHA256: hex64("3")}},
	}
}

func mustBuild(t testing.TB, f provenance.ReleaseFacts) []byte {
	t.Helper()
	b, err := provenance.Build(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// checkSchemas validates payload against both vendored schemas.
func checkSchemas(t *testing.T, payload []byte) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	validate(t, "testdata/in-toto-statement-v1.schema.json", doc)
	validate(t, "testdata/slsa-provenance-v1.schema.json", doc["predicate"])
}

func validate(t *testing.T, schemaFile string, instance any) {
	t.Helper()
	if err := schemaError(t, schemaFile, instance); err != nil {
		t.Fatalf("%s: %v", schemaFile, err)
	}
}

func schemaError(t *testing.T, schemaFile string, instance any) error {
	t.Helper()
	raw, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	rs, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return rs.Validate(instance)
}
