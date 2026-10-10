// Purpose: shared helpers for the values tests: load a fixture project from
// testdata/, run Resolve against its recorded `docker compose config` JSON,
// and compare with golden files (UPDATE_GOLDEN=1 rewrites them).
//
// Inputs: ../../testdata/<name>/ (see testdata/*/SOURCE).
//
// Outputs: helpers only.
//
// Constraints: no network, no docker; the recorded JSON stands in for compose.
package values

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureDir(name string) string { return filepath.Join("..", "..", "testdata", name) }

// recordedRunner returns the fixture's config.json and records the argv.
func recordedRunner(t *testing.T, name string, got *[]string) Runner {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir(name), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	return func(_ context.Context, _ string, argv []string) ([]byte, error) {
		if got != nil {
			*got = append([]string(nil), argv...)
		}
		return data, nil
	}
}

// mapFixture resolves and maps a fixture project.
func mapFixture(t *testing.T, name string) (*Model, *Values, *Secrets) {
	t.Helper()
	dir := fixtureDir(name)
	m, err := Resolve(context.Background(), dir, recordedRunner(t, name, nil))
	if err != nil {
		t.Fatal(err)
	}
	routes, err := LoadRoutes(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, s, err := Map(m, routes)
	if err != nil {
		t.Fatal(err)
	}
	return m, v, s
}

// compareGolden requires got to open with the GENERATED marker line and
// compares the rest with the golden. Goldens are stored without the marker:
// the repo's generated-file gate refuses any committed file that carries it.
func compareGolden(t *testing.T, name, file string, got []byte) {
	t.Helper()
	body, ok := strings.CutPrefix(string(got), GeneratedMarker+"\n")
	if !ok {
		t.Fatalf("%s/%s does not open with the GENERATED marker line", name, file)
	}
	got = []byte(body)
	path := filepath.Join(fixtureDir(name), "golden", file)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (UPDATE_GOLDEN=1 to create)", path, err)
	}
	if string(want) != string(got) {
		t.Errorf("%s/%s differs from golden (UPDATE_GOLDEN=1 to accept):\n%s", name, file, got)
	}
}
