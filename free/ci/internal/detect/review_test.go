package detect

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	_ "unsafe"

	_ "github.com/nself-org/plugins/free/ci/internal/legacy"
)

//go:linkname legacyDetectStacks github.com/nself-org/plugins/free/ci/internal/legacy.detectStacks
func legacyDetectStacks(string) []string

type unreadableManifest struct {
	fs.FS
	name string
}

func (u unreadableManifest) Open(name string) (fs.File, error) {
	if name == u.name {
		return nil, fs.ErrPermission
	}
	return u.FS.Open(name)
}

func TestManifestFailureIsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		bad  string
	}{
		{"go.mod", "invalid data\n"},
		{"Cargo.toml", "invalid data\n"},
		{"pubspec.yaml", "[invalid\n"},
		{"package.json", "{invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, source := range []struct {
				name string
				fs   fs.FS
			}{
				{"invalid", fstest.MapFS{tc.name: &fstest.MapFile{Data: []byte(tc.bad)}}},
				{"unreadable", unreadableManifest{fstest.MapFS{tc.name: &fstest.MapFile{Data: []byte(tc.bad)}}, tc.name}},
			} {
				t.Run(source.name, func(t *testing.T) {
					facts, err := DefaultRegistry().Detect(source.fs, allChecks)
					if err != nil {
						t.Fatal(err)
					}
					foundUnknown := false
					for _, f := range facts {
						if f.Kind == "stack" && f.Confidence == Certain {
							t.Fatalf("confident stack on failed manifest: %+v", f)
						}
						if f.Confidence == Unknown && f.Path == tc.name && f.Reason != "" {
							foundUnknown = true
						}
					}
					if !foundUnknown {
						t.Fatalf("missing reasoned unknown: %+v", facts)
					}
				})
			}
		})
	}
}

func TestManifestMarkersMustBeDirectives(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"go.mod", "// module example.org/comment\n"},
		{"Cargo.toml", "# [package]\n"},
		{"pubspec.yaml", "{}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := DefaultRegistry().Detect(fstest.MapFS{tc.name: &fstest.MapFile{Data: []byte(tc.body)}}, allChecks)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range facts {
				if f.Kind == "stack" && f.Confidence == Certain {
					t.Fatalf("marker only yielded stack: %+v", facts)
				}
			}
		})
	}
}

func TestUnreadableDockerfileIsUnknown(t *testing.T) {
	source := unreadableManifest{fstest.MapFS{"Dockerfile": &fstest.MapFile{Data: []byte("FROM alpine:3.21\n")}}, "Dockerfile"}
	facts, err := DefaultRegistry().Detect(source, allChecks)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range facts {
		if f.Kind == "container" && f.Confidence == Certain {
			t.Fatalf("unreadable Dockerfile yielded container: %+v", f)
		}
		if f.Kind == "container" && f.Confidence == Unknown && f.Path == "Dockerfile" && f.Reason != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing unknown container: %+v", facts)
	}
}

func TestLegacyFixtureStackCoverage(t *testing.T) {
	for _, name := range []string{"go", "node-npm-workspace", "node-pnpm-workspace", "flutter", "rust", "polyglot", "monorepo"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join("testdata", name)
			if name == "go" || name == "flutter" {
				root = t.TempDir()
				manifest := "go.mod"
				if name == "flutter" {
					manifest = "pubspec.yaml"
				}
				b, err := os.ReadFile(filepath.Join("testdata", name, manifest+".fixture"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, manifest), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			legacy := legacyDetectStacks(root)
			if len(legacy) == 0 {
				t.Fatal("vacuous fixture: legacy detected no stack")
			}
			facts, err := DefaultRegistry().Detect(os.DirFS(root), allChecks)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range legacy {
				found := false
				for _, f := range facts {
					if f.Kind == "stack" && f.Name == want && f.Confidence == Certain {
						found = true
					}
				}
				if !found {
					t.Fatalf("legacy stack %q missing: %+v", want, facts)
				}
			}
		})
	}
}
