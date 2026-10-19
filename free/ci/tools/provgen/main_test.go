package main

// Purpose: tests of provgen (P7-CACHE-10): its statement validates against the vendored in-toto and SLSA schemas, parses with
//          internal/provenance, binds the binary and both SBOMs, and is the same bytes every time; it refuses a wrong digest.
// Inputs:  small files written in the test.
// Constraints: no network, no key.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

const workflowRef = "nself-org/plugins/.github/workflows/release-agent.yml@refs/tags/v1.5.0"

type inputs struct {
	dir, binary, sha, cdx, spdx string
}

func newInputs(t *testing.T) inputs {
	t.Helper()
	dir := t.TempDir()
	in := inputs{dir: dir, binary: filepath.Join(dir, "nself-ci-agent-linux-amd64"), cdx: filepath.Join(dir, "nself-ci-agent-linux-amd64.cdx.json"),
		spdx: filepath.Join(dir, "nself-ci-agent-linux-amd64.spdx.json")}
	for p, body := range map[string]string{in.binary: "agent bytes", in.cdx: `{"bomFormat":"CycloneDX"}`, in.spdx: `{"spdxVersion":"SPDX-2.3"}`} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := sha256.Sum256([]byte("agent bytes"))
	in.sha = hex.EncodeToString(h[:])
	return in
}

func (in inputs) args(out string) []string {
	return []string{"--binary", in.binary, "--sha256", in.sha, "--sbom", in.cdx, "--sbom", in.spdx, "--builder-id", workflowRef,
		"--revision", strings.Repeat("a", 40), "--run-id", "42", "--out", out}
}

func gen(t *testing.T, args []string) (code int, out, errs string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, &o, &e)
	return code, o.String(), e.String()
}

func validate(t *testing.T, schemaFile string, instance any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "provenance", "testdata", schemaFile))
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
	if err = rs.Validate(instance); err != nil {
		t.Fatalf("%s: %v", schemaFile, err)
	}
}

func TestProvgenWritesAValidStatementAndItsPAE(t *testing.T) {
	in := newInputs(t)
	out := filepath.Join(t.TempDir(), "p")
	if code, stdout, errs := gen(t, in.args(out)); code != 0 || !strings.HasPrefix(stdout, "ok nself-ci-agent-linux-amd64 "+in.sha) {
		t.Fatalf("code %d out %q err %q", code, stdout, errs)
	}
	statement, err := os.ReadFile(filepath.Join(out, "statement.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(statement, &doc); err != nil {
		t.Fatal(err)
	}
	validate(t, "in-toto-statement-v1.schema.json", doc)
	validate(t, "slsa-provenance-v1.schema.json", doc["predicate"])
	st, err := provenance.Parse(statement)
	if err != nil {
		t.Fatal(err)
	}
	if st.Predicate.BuildDefinition.BuildType != provenance.BuildTypeAgent || st.SubjectDigests()["nself-ci-agent-linux-amd64"] != in.sha {
		t.Fatalf("statement does not name the agent binary: %+v", st)
	}
	if st.Predicate.RunDetails.Builder.ID != "https://github.com/nself-org/plugins/"+provenance.AgentWorkflow+"@refs/tags/v1.5.0" ||
		st.Predicate.RunDetails.Metadata.InvocationID != "https://github.com/nself-org/plugins/actions/runs/42" {
		t.Fatalf("builder or invocation: %+v", st.Predicate.RunDetails)
	}
	for _, name := range []string{"nself-ci-agent-linux-amd64.cdx.json", "nself-ci-agent-linux-amd64.spdx.json"} {
		if d, ok := st.Byproduct(name); !ok || len(d) != 64 {
			t.Fatalf("byproduct %s missing", name)
		}
	}
	pae, err := os.ReadFile(filepath.Join(out, "statement.pae"))
	if err != nil || !bytes.Equal(pae, signing.PAE(provenance.PayloadType, statement)) {
		t.Fatalf("statement.pae is not the DSSE PAE of the statement: %v", err)
	}
}

func TestProvgenIsDeterministic(t *testing.T) {
	in := newInputs(t)
	a, b := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	if c, _, e := gen(t, in.args(a)); c != 0 {
		t.Fatal(e)
	}
	if c, _, e := gen(t, in.args(b)); c != 0 {
		t.Fatal(e)
	}
	for _, f := range []string{"statement.json", "statement.pae"} {
		x, _ := os.ReadFile(filepath.Join(a, f))
		y, _ := os.ReadFile(filepath.Join(b, f))
		if len(x) == 0 || !bytes.Equal(x, y) {
			t.Fatalf("%s differs between runs", f)
		}
	}
}

func TestProvgenRefusals(t *testing.T) {
	in := newInputs(t)
	set := func(a []string, flag, v string) []string {
		out := append([]string(nil), a...)
		for i := range out {
			if out[i] == flag {
				out[i+1] = v
				break
			}
		}
		return out
	}
	existing := t.TempDir()
	cases := map[string][]string{
		"digest of another binary": set(in.args(filepath.Join(t.TempDir(), "o")), "--sha256", strings.Repeat("b", 64)),
		"another workflow":         set(in.args(filepath.Join(t.TempDir(), "o")), "--builder-id", "nself-org/plugins/.github/workflows/other.yml@refs/tags/v1"),
		"bad run id":               set(in.args(filepath.Join(t.TempDir(), "o")), "--run-id", "forty-two"),
		"short revision":           set(in.args(filepath.Join(t.TempDir(), "o")), "--revision", "abc"),
		"missing SBOM file":        set(in.args(filepath.Join(t.TempDir(), "o")), "--sbom", filepath.Join(in.dir, "absent.json")),
		"output directory exists":  in.args(existing),
	}
	for name, args := range cases {
		if code, _, errs := gen(t, args); code != 1 || errs == "" {
			t.Errorf("%s: code %d err %q", name, code, errs)
		}
	}
	link := filepath.Join(in.dir, "link")
	if err := os.Symlink(in.binary, link); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := gen(t, set(in.args(filepath.Join(t.TempDir(), "o")), "--binary", link)); code != 1 {
		t.Error("a symlinked binary must be refused")
	}
}

func TestProvgenUsage(t *testing.T) {
	if code, _, _ := gen(t, nil); code != 2 {
		t.Error("no arguments is a usage error")
	}
	in := newInputs(t)
	if code, _, _ := gen(t, append(in.args(filepath.Join(t.TempDir(), "o")), "extra")); code != 2 {
		t.Error("a stray argument is a usage error")
	}
}
