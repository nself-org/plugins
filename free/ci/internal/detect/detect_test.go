package detect

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func allChecks(string) bool { return true }

func TestGoldenFixtures(t *testing.T) {
	for _, name := range []string{"monorepo", "polyglot", "compose", "node-pg", "empty"} {
		t.Run(name, func(t *testing.T) {
			facts, err := DefaultRegistry().Detect(os.DirFS(filepath.Join("testdata", name)), allChecks)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(facts, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			golden := filepath.Join("testdata", name+".golden.json")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(golden, got, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("golden mismatch:\n%s", got)
			}
			if name == "node-pg" {
				for _, f := range facts {
					if f.Kind == "service" {
						t.Fatal("client dependency inferred service")
					}
				}
			}
			if name == "empty" && (len(facts) != 1 || facts[0].Name != "no_stack") {
				t.Fatalf("empty repo: %+v", facts)
			}
		})
	}
}

func TestUnboundDetectorIsSilent(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Detector{ID: "future", Ecosystem: "python", CheckIDs: []string{"python.test"}, Detect: func(Snapshot) []Fact { return []Fact{fact("stack", "python", "pyproject.toml")} }}); err != nil {
		t.Fatal(err)
	}
	facts, err := r.Detect(fstest.MapFS{"pyproject.toml": &fstest.MapFile{Data: []byte("[project]")}}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].Name != "no_stack" {
		t.Fatalf("unbound emitted facts: %+v", facts)
	}
	if err := r.Register(Detector{ID: "future", Ecosystem: "python", CheckIDs: []string{"python.test"}, Detect: func(Snapshot) []Fact { return nil }}); err == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestSnapshotLimitsAndIgnored(t *testing.T) {
	facts, err := DefaultRegistry().Detect(fstest.MapFS{
		"vendor/go.mod":      &fstest.MapFile{Data: []byte("module ignored")},
		"a/b/c/d/e/f/go.mod": &fstest.MapFile{Data: []byte("module too-deep")},
	}, allChecks)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].Name != "no_stack" {
		t.Fatalf("ignored paths yielded %+v", facts)
	}
}

func TestSymlinkIsNotRead(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(outside, []byte("module outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "go.mod")); err != nil {
		t.Skip(err)
	}
	facts, err := DefaultRegistry().Detect(os.DirFS(dir), allChecks)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].Name != "no_stack" {
		t.Fatalf("symlink read: %+v", facts)
	}
}

func TestLegacyStackCoverage(t *testing.T) {
	for marker, want := range map[string]string{"go.mod": "go", "package.json": "node", "pubspec.yaml": "flutter", "Cargo.toml": "rust"} {
		data := []byte("{}")
		if marker == "pubspec.yaml" {
			data = []byte("dependencies:\n  flutter:\n    sdk: flutter\n")
		}
		facts, err := DefaultRegistry().Detect(fstest.MapFS{marker: &fstest.MapFile{Data: data}}, allChecks)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range facts {
			if f.Kind == "stack" && f.Name == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s missing stack %s: %+v", marker, want, facts)
		}
	}
}

func TestNoCodeExecution(t *testing.T) {
	// A project script and an environment sample are inert snapshot data.
	files := fstest.MapFS{"package.json": &fstest.MapFile{Data: []byte(`{"scripts":{"test":"touch /tmp/nope"},"dependencies":{"pg":"1"}}`)}, ".env.example": &fstest.MapFile{Data: []byte("POSTGRES_URL=x")}}
	facts, err := DefaultRegistry().Detect(files, allChecks)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range facts {
		if f.Kind == "service" {
			t.Fatal(f)
		}
	}
}

func TestAmbiguousPackageManager(t *testing.T) {
	files := fstest.MapFS{
		"package.json":   &fstest.MapFile{Data: []byte(`{"name":"x"}`)},
		"pnpm-lock.yaml": &fstest.MapFile{Data: []byte("x")},
		"yarn.lock":      &fstest.MapFile{Data: []byte("x")},
	}
	facts, err := DefaultRegistry().Detect(files, allChecks)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range facts {
		if f.Kind == "package_manager" && f.Confidence != Unknown {
			t.Fatalf("guessed manager: %+v", f)
		}
	}
}

func TestWorkspaceExclusion(t *testing.T) {
	files := fstest.MapFS{
		"package.json":               &fstest.MapFile{Data: []byte(`{"workspaces":["packages/*","!packages/skip"]}`)},
		"packages/keep/package.json": &fstest.MapFile{Data: []byte(`{}`)},
		"packages/skip/package.json": &fstest.MapFile{Data: []byte(`{}`)},
	}
	facts, err := DefaultRegistry().Detect(files, allChecks)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, f := range facts {
		if f.Kind == "workspace_member" {
			count++
			if f.Name != "packages/keep" {
				t.Fatalf("excluded member: %+v", f)
			}
		}
	}
	if count != 1 {
		t.Fatalf("want one member, got %d", count)
	}
}

func TestDeclarativeToolEvidence(t *testing.T) {
	files := fstest.MapFS{
		"package.json":  &fstest.MapFile{Data: []byte(`{"scripts":{"test":"pnpm exec vitest run","lint":"echo eslint"}}`)},
		"tsconfig.json": &fstest.MapFile{Data: []byte(`{}`)},
	}
	facts, err := DefaultRegistry().Detect(files, allChecks)
	if err != nil {
		t.Fatal(err)
	}
	vitest, tsc, eslint := false, false, false
	for _, f := range facts {
		if f.Name == "vitest" {
			vitest = true
		}
		if f.Name == "tsc" {
			tsc = true
		}
		if f.Name == "eslint" {
			eslint = true
		}
	}
	if !vitest || !tsc || eslint {
		t.Fatalf("tool evidence: %+v", facts)
	}
}

func FuzzPackageJSON(f *testing.F) { fuzzFile(f, "package.json", `{"scripts":{"test":"vitest"}}`) }
func FuzzGoMod(f *testing.F)       { fuzzFile(f, "go.mod", "module example.org/x\n") }
func FuzzCargo(f *testing.F)       { fuzzFile(f, "Cargo.toml", "[package]\nname='x'\n") }
func FuzzCompose(f *testing.F) {
	fuzzFile(f, "compose.yaml", "services:\n  db:\n    image: postgres:16\n")
}

func fuzzFile(f *testing.F, name, seed string) {
	f.Add([]byte(seed))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		facts, err := DefaultRegistry().Detect(fstest.MapFS{name: &fstest.MapFile{Data: data}}, allChecks)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range facts {
			if x.Path == "" || strings.Contains(x.Path, "..") {
				t.Fatalf("invalid path %+v", x)
			}
		}
	})
}

var _ fs.FS = fstest.MapFS{}
