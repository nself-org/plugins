package legacy

// Package internal — gate_modules_test.go
//
// Purpose: Table tests for goModuleDirs and Go stack detection with nested
//   modules (D-0922).

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGoModuleDirs(t *testing.T) {
	cases := []struct {
		name       string
		files      []string
		wantMods   []string
		wantStacks []string
	}{
		{"root go.mod only", []string{"go.mod"}, []string{"."}, []string{"go"}},
		{"rust root with nested go", []string{"Cargo.toml", "paid/b/go.mod", "paid/a/go.mod"},
			[]string{"paid/a", "paid/b"}, []string{"go", "rust"}},
		{"vendor ignored", []string{"vendor/x/go.mod", "Cargo.toml"}, nil, []string{"rust"}},
		{"node_modules ignored", []string{"node_modules/x/go.mod", "Cargo.toml"}, nil, []string{"rust"}},
		{"testdata ignored", []string{"a/testdata/go.mod", "testdata/go.mod", "Cargo.toml"}, nil, []string{"rust"}},
		{"dot dir ignored", []string{".github/x/go.mod", "Cargo.toml"}, nil, []string{"rust"}},
		{"depth 3 ignored", []string{"a/b/c/go.mod", "Cargo.toml"}, nil, []string{"rust"}},
		{"depth 1", []string{"svc/go.mod", "Cargo.toml"}, []string{"svc"}, []string{"go", "rust"}},
		{"nested without root Cargo.toml stays out", []string{"package.json", "free/a/go.mod", "sdk/go.mod"}, nil, []string{"node"}},
		{"root wins over nested", []string{"go.mod", "paid/a/go.mod"}, []string{"."}, []string{"go"}},
		{"empty repo", nil, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for _, f := range c.files {
				p := filepath.Join(root, filepath.FromSlash(f))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := goModuleDirs(root); !reflect.DeepEqual(got, c.wantMods) {
				t.Errorf("goModuleDirs = %v, want %v", got, c.wantMods)
			}
			if got := detectStacks(root); !reflect.DeepEqual(got, c.wantStacks) {
				t.Errorf("detectStacks = %v, want %v", got, c.wantStacks)
			}
		})
	}
}
