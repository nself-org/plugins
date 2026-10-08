package invariants

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCatalogCoverage(t *testing.T) {
	known := map[string]Invariant{}
	for _, row := range Catalog {
		if row.ID == "" || row.Owner == "" || row.Test != "TestInvariant_"+row.ID {
			t.Fatalf("invalid catalogue row %+v", row)
		}
		if _, ok := known[row.ID]; ok {
			t.Fatalf("duplicate %s", row.ID)
		}
		known[row.ID] = row
	}
	if len(known) != 19 {
		t.Fatalf("catalogue has %d entries", len(known))
	}
	root := "../.."
	found := map[string]bool{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			f, ok := decl.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(f.Name.Name, "TestInvariant_") {
				continue
			}
			id := strings.TrimPrefix(f.Name.Name, "TestInvariant_")
			if _, ok := known[id]; !ok {
				t.Errorf("uncatalogued invariant %s", id)
			}
			found[id] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("NSELF_CI_CATALOG_STRICT") == "1" {
		missing := []string{}
		for id := range known {
			if !found[id] {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Fatalf("missing invariant tests: %s", strings.Join(missing, ", "))
		}
	}
}
