// Purpose: tests for manifest reading and the compose argv.
//
// Inputs: testdata/full and temp project dirs.
//
// Outputs: test results.
//
// Constraints: no docker.
package values

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArgvFollowsManifestOrder(t *testing.T) {
	var got []string
	dir := fixtureDir("full")
	if _, err := Resolve(context.Background(), dir, recordedRunner(t, "full", &got)); err != nil {
		t.Fatal(err)
	}
	want := []string{"docker", "compose",
		"--env-file", filepath.Join(dir, ".env"), "--env-file", filepath.Join(dir, ".nself/compose.env"),
		"-f", filepath.Join(dir, "docker-compose.yml"), "config", "--format", "json"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv =\n%v\nwant\n%v", got, want)
	}
}

func TestNoSecretOnArgv(t *testing.T) {
	var got []string
	if _, err := Resolve(context.Background(), fixtureDir("interp"), recordedRunner(t, "interp", &got)); err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if strings.Contains(a, "fixture-") {
			t.Errorf("argv carries a secret value: %q", a)
		}
	}
}

func TestMissingEnvManifestFailsClosed(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".nself/compose-files.txt"), "docker-compose.yml\n")
	mustWrite(t, filepath.Join(dir, ".nself/compose.env"), "A=b\n")
	_, _, err := ReadManifests(dir)
	if err == nil || !strings.Contains(err.Error(), "run nself build") {
		t.Fatalf("want a run-nself-build error, got %v", err)
	}
}

func TestDefaultsWithoutManifests(t *testing.T) {
	dir := t.TempDir()
	files, envs, err := ReadManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "docker-compose.yml" || len(envs) != 0 {
		t.Errorf("files=%v envs=%v", files, envs)
	}
}

func TestUserOverrideAppended(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".nself/compose-files.txt"), "docker-compose.yml\n")
	mustWrite(t, filepath.Join(dir, "docker-compose.override.yml"), "services: {}\n")
	files, _, err := ReadManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || filepath.Base(files[1]) != "docker-compose.override.yml" {
		t.Errorf("files = %v", files)
	}
}

func TestParseModelErrorsNeverEchoInput(t *testing.T) {
	_, err := ParseModel([]byte(`{"services": "super-secret-value"`))
	if err == nil || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ParseModel([]byte(`{"services":{}}`)); err == nil {
		t.Fatal("want an error for no services")
	}
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
