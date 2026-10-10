// Purpose: tests for Run, the generate and --check behaviour of
// `nself-k8s values`, over the full fixture with the recorded compose JSON.
//
// Inputs: testdata/full.
//
// Outputs: test results.
//
// Constraints: no docker; the project manifests are copied to a temp dir so
// the generated files never touch testdata.
package values

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runProject copies the fixture manifests to a temp project and returns it.
func runProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{".nself/compose-files.txt", ".nself/compose-env-files.txt", ".nself/generated/routes.json"} {
		b, err := os.ReadFile(filepath.Join(fixtureDir("full"), f))
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(dir, f), string(b))
	}
	return dir
}

func runOpts(t *testing.T, dir string, check bool, run Runner) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := Run(context.Background(), Options{ProjectDir: dir, Check: check, Runner: run, Out: &buf})
	return buf.String(), err
}

func TestRunWritesBothFiles(t *testing.T) {
	dir := runProject(t)
	out, err := runOpts(t, dir, false, recordedRunner(t, "full", nil))
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	gen := filepath.Join(dir, ".nself", "generated", "k8s")
	st, err := os.Stat(filepath.Join(gen, SecretsFile))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("secrets.yaml: %v %v", err, st)
	}
	want, _ := os.ReadFile(filepath.Join(fixtureDir("full"), "golden", "values.yaml"))
	got, _ := os.ReadFile(filepath.Join(gen, ValuesFile))
	if string(want) != string(got) {
		t.Error("values.yaml differs from the golden")
	}
	if !strings.Contains(out, "unsupported: worker-one") {
		t.Errorf("output should list unsupported services:\n%s", out)
	}
	if strings.Contains(out, "fixture-") {
		t.Errorf("output leaks a secret value:\n%s", out)
	}
}

func TestRunCheckPassesThenFailsNamingService(t *testing.T) {
	dir := runProject(t)
	run := recordedRunner(t, "full", nil)
	if _, err := runOpts(t, dir, false, run); err != nil {
		t.Fatal(err)
	}
	out, err := runOpts(t, dir, true, run)
	if err != nil || !strings.Contains(out, "parity ok") {
		t.Fatalf("check on fresh values: err=%v\n%s", err, out)
	}
	vf := filepath.Join(dir, ".nself", "generated", "k8s", ValuesFile)
	b, _ := os.ReadFile(vf)
	edited := strings.Replace(string(b), "tag: v2.44.0", "tag: v1.0.0", 1)
	if edited == string(b) {
		t.Fatal("fixture no longer carries hasura v2.44.0; update this test")
	}
	mustWrite(t, vf, edited)
	out, err = runOpts(t, dir, true, run)
	if err == nil || !strings.Contains(out, "hasura: differs") {
		t.Fatalf("want a failure naming hasura, got err=%v\n%s", err, out)
	}
}

func TestRunCheckNamesAddedComposeService(t *testing.T) {
	dir := runProject(t)
	run := recordedRunner(t, "full", nil)
	if _, err := runOpts(t, dir, false, run); err != nil {
		t.Fatal(err)
	}
	added := func(ctx context.Context, d string, argv []string) ([]byte, error) {
		b, err := run(ctx, d, argv)
		return bytes.Replace(b, []byte(`"services": {`), []byte(`"services": {"brand-new": {"image": "busybox:1.36"},`), 1), err
	}
	out, err := runOpts(t, dir, true, added)
	if err == nil || !strings.Contains(out, "brand-new: missing") {
		t.Fatalf("want a failure naming brand-new, got err=%v\n%s", err, out)
	}
}

func TestRunCheckFailsOnMissingValuesFile(t *testing.T) {
	dir := runProject(t)
	if _, err := runOpts(t, dir, true, recordedRunner(t, "full", nil)); err == nil {
		t.Fatal("want an error when values.yaml does not exist")
	}
}
