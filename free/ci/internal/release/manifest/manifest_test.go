package manifest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
		"empty repo":        func(m *manifest.Manifest) { m.RepoID = "" },
		"bad finished_at":   func(m *manifest.Manifest) { m.Release.FinishedAt = "yesterday" },
		"evidence path":     func(m *manifest.Manifest) { m.Evidence.Path = "../evidence.json" },
		"evidence not pass": func(m *manifest.Manifest) { m.Evidence.Result = "fail" },
		"evidence class":    func(m *manifest.Manifest) { m.Evidence.Class = "default" },
		"no artifacts":      func(m *manifest.Manifest) { m.Artifacts = nil },
		"dotdot name":       func(m *manifest.Manifest) { m.Artifacts[0].Name = "a/../b"; m.Artifacts[0].Path = "artifacts/a/../b" },
		"absolute name":     func(m *manifest.Manifest) { m.Artifacts[0].Name = "/etc/passwd" },
		"empty component":   func(m *manifest.Manifest) { m.Artifacts[0].Name = "a//b" },
		"path mismatch":     func(m *manifest.Manifest) { m.Artifacts[0].Path = "artifacts/other" },
		"duplicate name":    func(m *manifest.Manifest) { m.Artifacts[1] = m.Artifacts[0] },
		"case-folded name": func(m *manifest.Manifest) {
			m.Artifacts[1].Name = "NSELF-linux-amd64"
			m.Artifacts[1].Path = "artifacts/NSELF-linux-amd64"
			m.Artifacts[1].SBOM = sbomFor("NSELF-linux-amd64", "8")
		},
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
		"key id shape":       func(m *manifest.Manifest) { m.Signing.KeyID = "ci-release-0123" },
		"key id bidi":        func(m *manifest.Manifest) { m.Signing.KeyID = "ci-release-0123456789abcde\u202e" },
		"image ref spoof":    func(m *manifest.Manifest) { m.Artifacts[1].Image.Ref = "ghcr.io/e\u0430xample/nself:1" },
		"image ref space":    func(m *manifest.Manifest) { m.Artifacts[1].Image.Ref = "ghcr.io/example/nself 1" },
		"sbom file and directory": func(m *manifest.Manifest) {
			m.Artifacts[0].Name = "a" // sbom/a.cdx.json
			m.Artifacts[0].Path = "artifacts/a"
			m.Artifacts[0].SBOM = sbomFor("a", "3")
			m.Artifacts[1].Name = "a.cdx.json/b" // sbom/a.cdx.json/b.cdx.json
			m.Artifacts[1].Path = "artifacts/a.cdx.json/b"
			m.Artifacts[1].SBOM = sbomFor("a.cdx.json/b", "8")
		},
		"file and directory": func(m *manifest.Manifest) {
			m.Artifacts[0].Name = "image" // image/nself is the other artifact
			m.Artifacts[0].Path = "artifacts/image"
			m.Artifacts[0].SBOM = sbomFor("image", "3")
		},
		"empty coordinator": func(m *manifest.Manifest) { m.Producer.Coordinator = "" },
		"wrong schema":      func(m *manifest.Manifest) { m.Schema = "ci.release-manifest/v2" },
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
	f.Add([]byte("{\"schema\":\"ci.release-manifest/v2\",\"\u017fchema\":\"ci.release-manifest/v1\"}"))
	f.Fuzz(func(t *testing.T, in []byte) {
		m, err := manifest.Decode(in)
		if err != nil {
			return
		}
		var other map[string]any // a case-sensitive reader (jq, Python, TypeScript) must see the same values
		if err = json.Unmarshal(in, &other); err != nil {
			t.Fatalf("Decode accepted what a generic reader cannot parse: %v", err)
		}
		if other["schema"] != m.Schema || other["revision"] != m.Revision || other["repo_id"] != m.RepoID {
			t.Fatalf("Decode and a generic reader disagree: %v %v %v", other["schema"], other["revision"], other["repo_id"])
		}
		if sg, _ := other["signing"].(map[string]any); sg["key_id"] != m.Signing.KeyID {
			t.Fatalf("key_id: Decode %q, generic reader %v", m.Signing.KeyID, sg["key_id"])
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

func TestDecodeRefusesCaseVariantsAndMissingFields(t *testing.T) {
	good := string(mustBuild(t, valid()))
	rev := strings.Repeat("a", 40)
	cases := map[string]string{
		"case duplicate":        strings.Replace(good, `"revision":"`+rev+`"`, `"revision":"`+rev+`","Revision":"`+strings.Repeat("b", 40)+`"`, 1),
		"schema case duplicate": strings.Replace(good, `"schema":"ci.release-manifest/v1"`, `"schema":"ci.release-manifest/v2","SCHEMA":"ci.release-manifest/v1"`, 1),
		"lone case variant":     strings.Replace(good, `"repo_id"`, `"REPO_ID"`, 1),
		"kelvin sign alias":     strings.Replace(good, `"key_id"`, "\"\u212aey_id\"", 1),
		"long s alias":          strings.Replace(good, `"schema":"ci.release-manifest/v1"`, `"schema":"ci.release-manifest/v2","\u017fchema":"ci.release-manifest/v1"`, 1),
		"long s lone alias":     strings.Replace(good, `"sha256"`, "\"\u017fha256\"", 1),
		"missing size":          strings.Replace(good, `,"size":99`, ``, 1),
		"null size":             strings.Replace(good, `"size":99`, `"size":null`, 1),
		"missing platform":      strings.Replace(good, `"platform":null,`, ``, 1),
		"missing sbom":          strings.Replace(good, `"sbom":`, `"sbomx":`, 1),
		"null provenance":       strings.Replace(good, `"provenance":{`, `"provenance":null,"x":{`, 1),
		"string artifact":       strings.Replace(good, `"artifacts":[{`, `"artifacts":["x",{`, 1),
		"object for string":     strings.Replace(good, `"repo_id":"repo-1"`, `"repo_id":{}`, 1),
		"control character":     strings.Replace(good, `"repo-1"`, `"repo\u0000-1"`, 1),
		"newline in key id":     strings.Replace(good, `ci-release-0123456789abcdef`, `ci-release\n0123456789abcdef`, 1),
		"invalid utf-8":         strings.Replace(good, `"repo-1"`, "\"repo\xff-1\"", 1),
	}
	for name, doc := range cases {
		if doc == good {
			t.Errorf("%s: the case did not change the document", name)
		}
		if _, err := manifest.Decode([]byte(doc)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// a case variant of an UNKNOWN additive field is fine
	extra := strings.Replace(good, `"repo_id"`, `"Labels":1,"labels":2,"repo_id"`, 1)
	if _, err := manifest.Decode([]byte(extra)); err == nil {
		t.Error("two spellings of one additive field must still be refused")
	}
	if _, err := manifest.Decode([]byte(strings.Replace(good, `"repo_id"`, `"Labels":1,"repo_id"`, 1))); err != nil {
		t.Errorf("an additive field with capitals: %v", err)
	}
}

func TestBuildRefusesWhatDecodeCouldNotRead(t *testing.T) {
	m := valid()
	m.Artifacts = m.Artifacts[:1]
	m.Artifacts[0].Name = "a"
	m.Artifacts[0].Path = "artifacts/a"
	m.Artifacts[0].SBOM = sbomFor("a", "3")
	var arts []manifest.Artifact
	for i := 0; i < 4096; i++ {
		a := m.Artifacts[0]
		a.Name = strings.Repeat("n", 190) + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676))
		a.Path = "artifacts/" + a.Name
		a.SBOM = sbomFor(a.Name, "3")
		arts = append(arts, a)
	}
	m.Artifacts = arts
	if _, err := manifest.Build(m); !errors.Is(err, manifest.ErrInvalid) || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("an oversized payload was built: %v", err)
	}
	m.Artifacts = arts[:10]
	if b, err := manifest.Build(m); err != nil || len(b) > 1<<20 {
		t.Fatalf("a normal payload: %v", err)
	}
}

// TestSchemaRejectsWhatGoRejects: the rules the schema can express agree with
// Build and Decode (parity, review of 2026-10-19).
func TestSchemaRejectsWhatGoRejects(t *testing.T) {
	rs := resolvedSchema(t)
	base := func() map[string]any {
		var doc map[string]any
		if err := json.Unmarshal(mustBuild(t, valid()), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	art := func(d map[string]any, i int) map[string]any { return d["artifacts"].([]any)[i].(map[string]any) }
	bend := map[string]func(map[string]any){
		"dot component":       func(d map[string]any) { art(d, 0)["name"] = "a/../b" },
		"empty component":     func(d map[string]any) { art(d, 0)["name"] = "a//b" },
		"dot name":            func(d map[string]any) { art(d, 0)["name"] = ".." },
		"trailing slash":      func(d map[string]any) { art(d, 0)["name"] = "a/" },
		"path outside":        func(d map[string]any) { art(d, 0)["path"] = "evil/x" },
		"sbom path":           func(d map[string]any) { art(d, 0)["sbom"].(map[string]any)["spdx"].(map[string]any)["path"] = "x.json" },
		"image kind no image": func(d map[string]any) { art(d, 0)["image"] = nil },
		"binary with image":   func(d map[string]any) { art(d, 1)["image"] = art(d, 0)["image"] },
		"control character":   func(d map[string]any) { d["repo_id"] = "a\u0000b" },
		"long text":           func(d map[string]any) { d["repo_id"] = strings.Repeat("a", 513) },
		"no manifests":        func(d map[string]any) { art(d, 0)["image"].(map[string]any)["manifests"] = []any{} },
		"bad evidence class":  func(d map[string]any) { d["evidence"].(map[string]any)["class"] = "default" },
	}
	for name, f := range bend {
		d := base()
		f(d)
		if err := rs.Validate(d); err == nil {
			t.Errorf("%s: the schema accepted it", name)
		}
	}
	if err := rs.Validate(base()); err != nil {
		t.Fatalf("the valid document: %v", err)
	}
	d := base()
	art(d, 0)["name"] = "dir/.hidden/x..y"
	art(d, 0)["path"] = "artifacts/dir/.hidden/x..y"
	if err := rs.Validate(d); err != nil {
		t.Fatalf("a legal dotted name: %v", err)
	}
}

// TestTextRulesAgreeBetweenGoAndSchema: length counts characters and control
// characters (Unicode Cc) are refused by both.
func TestTextRulesAgreeBetweenGoAndSchema(t *testing.T) {
	rs := resolvedSchema(t)
	for name, c := range map[string]struct {
		repo string
		ok   bool
	}{
		"512 two-byte characters": {strings.Repeat("é", 512), true},
		"513 two-byte characters": {strings.Repeat("é", 513), false},
		"DEL":                     {"a\u007fb", false},
		"C1 control":              {"a\u0085b", false},
		"NUL":                     {"a\u0000b", false},
		"plain":                   {"repo-1", true},
	} {
		m := valid()
		m.RepoID = c.repo
		_, err := manifest.Build(m)
		var doc map[string]any
		if e := json.Unmarshal(mustBuild(t, valid()), &doc); e != nil {
			t.Fatal(e)
		}
		doc["repo_id"] = c.repo
		serr := rs.Validate(doc)
		if (err == nil) != c.ok || (serr == nil) != c.ok {
			t.Errorf("%s: Build %v, schema %v, want ok=%v", name, err, serr, c.ok)
		}
	}
}

// TestDecodeRefusesWhatBuildCannotWriteBack: U+2028 is three bytes raw and six
// escaped, so a payload under the cap can exceed it once canonical.
func TestDecodeRefusesWhatBuildCannotWriteBack(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(mustBuild(t, valid()), &doc); err != nil {
		t.Fatal(err)
	}
	var mfs []any
	for i := 0; i < 9000; i++ {
		mfs = append(mfs, map[string]any{"digest": "sha256:" + hex64("6"), "platform": fmt.Sprintf("p%05d-", i) + strings.Repeat(string(rune(0x2028)), 100)})
	}
	doc["artifacts"].([]any)[0].(map[string]any)["image"].(map[string]any)["manifests"] = mfs
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(doc)
	raw := bytes.ReplaceAll(buf.Bytes(), []byte("\\"+"u2028"), []byte(string(rune(0x2028)))) // encoding/json escapes it; write it raw
	if len(raw) >= 4<<20 {
		t.Fatalf("fixture too large: %d", len(raw))
	}
	if _, err := manifest.Decode(raw); !errors.Is(err, manifest.ErrInvalid) {
		t.Fatalf("Decode accepted a payload Build cannot write back: %v", err)
	}
}
