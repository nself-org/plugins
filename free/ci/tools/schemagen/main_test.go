package main

// Purpose: guards the schemagen registry (D-1079). Every schema is produced
// by a registry entry that main() walks; nothing writes from init().
// Constraints: the committed files in ../../schemas are the golden output.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const schemasDir = "../../schemas"

func TestRegistryCoversEveryCommittedSchema(t *testing.T) {
	entries, err := os.ReadDir(schemasDir)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, e := range entries {
		want = append(want, e.Name())
	}
	var got []string
	for name := range registry() {
		got = append(got, name)
	}
	sort.Strings(want)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("registry names %v, committed schemas %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("registry names %v, committed schemas %v", got, want)
		}
	}
}

func TestRegistryCoversEveryModelSchema(t *testing.T) {
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	reg := registry()
	for name := range schemas {
		if _, ok := reg[name]; !ok {
			t.Errorf("model.Schemas() produces %s but the registry has no entry", name)
		}
	}
}

func TestGenerateMatchesCommittedSchemas(t *testing.T) {
	dir := t.TempDir()
	if err := generate(dir); err != nil {
		t.Fatal(err)
	}
	for name := range registry() {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(schemasDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the committed schema; run `go generate ./tools/schemagen/...`", name)
		}
	}
}

// TestNoInitWrites keeps schema writing in main(): an init() runs before the
// registry and hides a second write path.
func TestNoInitWrites(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "init" && fn.Recv == nil {
				t.Errorf("%s declares init(); register the schema in registry() instead", f)
			}
		}
	}
}
