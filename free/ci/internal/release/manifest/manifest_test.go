package manifest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func goldens(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "golden", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files (%v)", err)
	}
	return files
}

// TestGoldenRoundTrip: Decode then Build gives back the golden bytes.
func TestGoldenRoundTrip(t *testing.T) {
	if *update {
		writeGolden(t, "full.json", valid())
		min := valid()
		min.Artifacts = min.Artifacts[:1]
		writeGolden(t, "minimal.json", min)
	}
	for _, f := range goldens(t) {
		want, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		m, err := manifest.Decode(want)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		got, err := manifest.Build(m)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s does not round-trip:\n got %s\nwant %s", f, got, want)
		}
	}
}

func writeGolden(t *testing.T, name string, m manifest.Manifest) {
	t.Helper()
	if err := os.WriteFile(filepath.Join("testdata", "golden", name), mustBuild(t, m), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildIsDeterministicAndSorted(t *testing.T) {
	a := mustBuild(t, valid())
	rev := valid()
	rev.Artifacts[0], rev.Artifacts[1] = rev.Artifacts[1], rev.Artifacts[0]
	if b := mustBuild(t, rev); !bytes.Equal(a, b) {
		t.Fatal("artifact order changed the bytes")
	}
	if !strings.Contains(string(a), `"artifacts":[{"image":{"index_digest"`) {
		t.Fatalf("artifacts are not sorted by name with sorted keys: %s", a)
	}
	if i, j := strings.Index(string(a), `"platform":"linux/amd64"`), strings.Index(string(a), `"platform":"linux/arm64"`); i < 0 || j < 0 || i > j {
		t.Fatal("image manifests are not sorted by platform")
	}
	if bytes.ContainsAny(a, " \n\t") {
		t.Fatal("payload carries insignificant whitespace")
	}
	m := valid()
	before := m.Artifacts[0].Name
	mustBuild(t, m)
	if m.Artifacts[0].Name != before {
		t.Fatal("Build mutated its argument")
	}
}

func TestBuildKeepsLargeSizesExact(t *testing.T) {
	m := valid()
	m.Artifacts[0].Size = 9007199254740993 // 2^53 + 1
	got, err := manifest.Decode(mustBuild(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if got.Artifacts[1].Size != 9007199254740993 && got.Artifacts[0].Size != 9007199254740993 {
		t.Fatal("a size above 2^53 lost precision")
	}
}

func TestDecodeRefusesAnotherMajor(t *testing.T) {
	var tree map[string]any
	if err := json.Unmarshal(mustBuild(t, valid()), &tree); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{"ci.release-manifest/v2", "ci.release-manifest/v10", "ci.release-manifest/v1.1", "ci.evidence/v1", ""} {
		tree["schema"] = schema
		b, _ := json.Marshal(tree)
		if _, err := manifest.Decode(b); !errors.Is(err, manifest.ErrSchema) {
			t.Fatalf("schema %q: err = %v, want ErrSchema", schema, err)
		}
	}
}

func TestDecodeAcceptsUnknownAdditiveFields(t *testing.T) {
	var tree map[string]any
	if err := json.Unmarshal(mustBuild(t, valid()), &tree); err != nil {
		t.Fatal(err)
	}
	tree["future"] = map[string]any{"a": 1}
	tree["release"].(map[string]any)["extra"] = "x"
	tree["artifacts"].([]any)[0].(map[string]any)["labels"] = []any{"x"}
	b, _ := json.Marshal(tree)
	m, err := manifest.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, mustDecode(t, mustBuild(t, valid()))) {
		t.Fatal("unknown fields changed the known ones")
	}
}

func mustDecode(t testing.TB, b []byte) manifest.Manifest {
	t.Helper()
	m, err := manifest.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDecodeRefusesMalformedPayloads(t *testing.T) {
	good := string(mustBuild(t, valid()))
	cases := map[string]string{
		"empty":              ``,
		"array":              `[]`,
		"trailing data":      good + `{}`,
		"repeated key":       strings.Replace(good, `"repo_id":"repo-1"`, `"repo_id":"repo-1","repo_id":"repo-2"`, 1),
		"repeated nested":    strings.Replace(good, `"purpose":"ci-release"`, `"purpose":"ci-release","purpose":"x"`, 1),
		"truncated":          good[:len(good)-5],
		"no schema":          `{"repo_id":"x"}`,
		"too deep":           `{"schema":"ci.release-manifest/v1","x":` + strings.Repeat(`[`, 40) + strings.Repeat(`]`, 40) + `}`,
		"oversized":          `{"schema":"ci.release-manifest/v1","x":"` + strings.Repeat("a", 5<<20) + `"}`,
		"wrong type":         strings.Replace(good, `"size":99`, `"size":"99"`, 1),
		"negative size":      strings.Replace(good, `"size":99`, `"size":-1`, 1),
		"short revision":     strings.Replace(good, strings.Repeat("a", 40), strings.Repeat("a", 39), 1),
		"uppercase revision": strings.Replace(good, strings.Repeat("a", 40), strings.Repeat("A", 40), 1),
		"null artifacts":     strings.Replace(good, `"artifacts":[`, `"artifacts":null,"x":[`, 1),
	}
	for name, doc := range cases {
		if _, err := manifest.Decode([]byte(doc)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidationRules(t *testing.T) {
	bend := map[string]func(*manifest.Manifest){
		"empty repo":         func(m *manifest.Manifest) { m.RepoID = "" },
		"bad finished_at":    func(m *manifest.Manifest) { m.Release.FinishedAt = "yesterday" },
		"evidence path":      func(m *manifest.Manifest) { m.Evidence.Path = "../evidence.json" },
		"evidence not pass":  func(m *manifest.Manifest) { m.Evidence.Result = "fail" },
		"evidence class":     func(m *manifest.Manifest) { m.Evidence.Class = "default" },
		"no artifacts":       func(m *manifest.Manifest) { m.Artifacts = nil },
		"dotdot name":        func(m *manifest.Manifest) { m.Artifacts[0].Name = "a/../b"; m.Artifacts[0].Path = "artifacts/a/../b" },
		"absolute name":      func(m *manifest.Manifest) { m.Artifacts[0].Name = "/etc/passwd" },
		"empty component":    func(m *manifest.Manifest) { m.Artifacts[0].Name = "a//b" },
		"path mismatch":      func(m *manifest.Manifest) { m.Artifacts[0].Path = "artifacts/other" },
		"duplicate name":     func(m *manifest.Manifest) { m.Artifacts[1] = m.Artifacts[0] },
		"unknown kind":       func(m *manifest.Manifest) { m.Artifacts[0].Kind = "rpm" },
		"short digest":       func(m *manifest.Manifest) { m.Artifacts[0].SHA256 = "abc" },
		"bad platform":       func(m *manifest.Manifest) { m.Artifacts[0].Platform = strp("linux") },
		"image without kind": func(m *manifest.Manifest) { m.Artifacts[0].Image = m.Artifacts[1].Image },
		"image kind no data": func(m *manifest.Manifest) { m.Artifacts[1].Image = nil },
		"index digest":       func(m *manifest.Manifest) { m.Artifacts[1].Image.IndexDigest = "sha1:abc" },
		"repeat platform":    func(m *manifest.Manifest) { m.Artifacts[1].Image.Manifests[1].Platform = "linux/arm64" },
		"no image manifests": func(m *manifest.Manifest) { m.Artifacts[1].Image.Manifests = nil },
		"sbom path":          func(m *manifest.Manifest) { m.Artifacts[0].SBOM.SPDX.Path = "sbom/other.spdx.json" },
		"provenance path":    func(m *manifest.Manifest) { m.Provenance.Path = "p.json" },
		"provenance level":   func(m *manifest.Manifest) { m.Provenance.SLSABuildLevel = "L3" },
		"checksums path":     func(m *manifest.Manifest) { m.Checksums.Path = "SUMS" },
		"signing purpose":    func(m *manifest.Manifest) { m.Signing.Purpose = "plugins" },
		"empty key id":       func(m *manifest.Manifest) { m.Signing.KeyID = "" },
		"empty coordinator":  func(m *manifest.Manifest) { m.Producer.Coordinator = "" },
		"wrong schema":       func(m *manifest.Manifest) { m.Schema = "ci.release-manifest/v2" },
	}
	for name, f := range bend {
		m := valid()
		f(&m)
		if _, err := manifest.Build(m); err == nil {
			t.Errorf("%s: Build accepted it", name)
		}
	}
	if _, err := manifest.Build(valid()); err != nil {
		t.Fatalf("the valid fixture was refused: %v", err)
	}
}

func resolvedSchema(t testing.TB) *jsonschema.Resolved {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "schemas", manifest.SchemaFile))
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err = json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	rs, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

// TestSchemaValidatesGoldensAndRejectsV2 runs the generated schema over the
// golden payloads and a v2 payload.
func TestSchemaValidatesGoldensAndRejectsV2(t *testing.T) {
	rs := resolvedSchema(t)
	for _, f := range goldens(t) {
		b, _ := os.ReadFile(f)
		var doc any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		if err := rs.Validate(doc); err != nil {
			t.Errorf("%s fails the schema: %v", f, err)
		}
		tree := doc.(map[string]any)
		tree["schema"] = "ci.release-manifest/v2"
		if err := rs.Validate(tree); err == nil {
			t.Errorf("%s: the schema accepted a v2 payload", f)
		}
	}
	var doc map[string]any
	_ = json.Unmarshal(mustBuild(t, valid()), &doc)
	doc["future_field"] = true
	if err := rs.Validate(doc); err != nil {
		t.Errorf("the schema refused an additive field: %v", err)
	}
	delete(doc, "signing")
	if err := rs.Validate(doc); err == nil {
		t.Error("the schema accepted a payload without signing")
	}
}

// TestSchemaDrift regenerates the schema and compares it with the file.
func TestSchemaDrift(t *testing.T) {
	want, err := manifest.JSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join("..", "..", "..", "schemas", manifest.SchemaFile))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("schemas/release-manifest.v1.schema.json is stale: run `go generate ./tools/schemagen/...`")
	}
	again, _ := manifest.JSONSchema()
	if !bytes.Equal(again, want) {
		t.Fatal("schema generation is not deterministic")
	}
}

// FuzzDecode: Decode never panics, and what it accepts is stable under
// Build -> Decode -> Build.
func FuzzDecode(f *testing.F) {
	for _, p := range []string{filepath.Join("testdata", "golden", "full.json"), filepath.Join("testdata", "golden", "minimal.json")} {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte(`{"schema":"ci.release-manifest/v2"}`))
	f.Add([]byte(`{"schema":"ci.release-manifest/v1","schema":"x"}`))
	f.Fuzz(func(t *testing.T, in []byte) {
		m, err := manifest.Decode(in)
		if err != nil {
			return
		}
		b1, err := manifest.Build(m)
		if err != nil {
			t.Fatalf("Decode accepted what Build refuses: %v", err)
		}
		m2, err := manifest.Decode(b1)
		if err != nil {
			t.Fatalf("Build output does not decode: %v", err)
		}
		b2, err := manifest.Build(m2)
		if err != nil || !bytes.Equal(b1, b2) {
			t.Fatalf("Build is not stable: %v", err)
		}
	})
}
