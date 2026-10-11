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
	"errors"
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
	res, err := Run(context.Background(), Options{ProjectDir: dir, Check: check, Runner: run})
	if res != nil {
		res.WriteText(&buf)
	}
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
	got, err := os.ReadFile(filepath.Join(gen, ValuesFile))
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, "full", "values.yaml", got)
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

// TestRunResultCarriesTheReport: Run returns the structured report both the
// human text and the --json document are built from, and prints nothing itself.
func TestRunResultCarriesTheReport(t *testing.T) {
	dir := runProject(t)
	run := recordedRunner(t, "full", nil)
	res, err := Run(context.Background(), Options{ProjectDir: dir, Runner: run})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != ModeGenerate || res.Mapped == 0 || len(res.Unsupported) != 2 || len(res.UnmappedRoutes) == 0 {
		t.Errorf("generate result = %+v", res)
	}
	if res.SecretsFile == "" || filepath.Base(res.ValuesFile) != ValuesFile {
		t.Errorf("files = %q %q", res.ValuesFile, res.SecretsFile)
	}
	if items := Items(res.Unsupported); len(items) != 2 || items[0].Reason == "" {
		t.Errorf("items = %+v", items)
	}
	chk, err := Run(context.Background(), Options{ProjectDir: dir, Check: true, Runner: run})
	if err != nil || chk.Mode != ModeCheck || chk.Services == 0 || len(chk.Diffs) != 0 {
		t.Fatalf("check result = %+v, err = %v", chk, err)
	}
	b, _ := os.ReadFile(res.ValuesFile)
	mustWrite(t, res.ValuesFile, strings.Replace(string(b), "tag: v2.44.0", "tag: v1.0.0", 1))
	chk, err = Run(context.Background(), Options{ProjectDir: dir, Check: true, Runner: run})
	var ve *Error
	if !errors.As(err, &ve) || ve.Kind != KindParity || chk == nil || len(chk.Diffs) != 1 {
		t.Fatalf("parity failure: result = %+v, err = %v", chk, err)
	}
	_, err = Run(context.Background(), Options{ProjectDir: dir, Runner: func(context.Context, string, []string) ([]byte, error) {
		return nil, errors.New("no docker")
	}})
	if !errors.As(err, &ve) || ve.Kind != KindCompose {
		t.Errorf("compose failure: err = %v", err)
	}
}
