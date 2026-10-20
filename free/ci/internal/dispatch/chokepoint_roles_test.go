package dispatch

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// A role Grant is unrelated to lease admission. Resolve its object rather than
// allowlisting files or functions, which could also contain a lease bypass.
func isRoleGrant(obj types.Object) bool {
	if obj == nil || obj.Name() != "Grant" || obj.Pkg() == nil {
		return false
	}
	const base = "github.com/nself-org/plugins/free/ci/internal/"
	switch obj.Pkg().Path() {
	case base + "store":
		_, ok := obj.(*types.TypeName)
		return ok
	case base + "roles":
		fn, ok := obj.(*types.Func)
		if !ok {
			return false
		}
		sig, ok := fn.Type().(*types.Signature)
		return ok && sig.Recv() != nil
	}
	return false
}

func roleGrantPositions(t *testing.T, root string) map[string]map[token.Pos]bool {
	t.Helper()
	result := map[string]map[token.Pos]bool{}
	// Planted modules deliberately contain unresolved identifiers. They retain
	// the conservative syntactic scan, including their lease Grant violations.
	if strings.Contains(filepath.ToSlash(root), "testdata/") {
		return result
	}
	pkgs, err := packages.Load(&packages.Config{Dir: root,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps}, "./...")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) != 0 {
			t.Fatalf("load %s: %v", pkg.PkgPath, pkg.Errors)
		}
		for _, objects := range []map[*ast.Ident]types.Object{pkg.TypesInfo.Uses, pkg.TypesInfo.Defs} {
			for id, obj := range objects {
				if !isRoleGrant(obj) {
					continue
				}
				pos := pkg.Fset.Position(id.Pos())
				rel, err := filepath.Rel(root, pos.Filename)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Clean(filepath.Join(root, rel))
				if result[path] == nil {
					result[path] = map[token.Pos]bool{}
				}
				// scanChokepoint parses each file with a fresh FileSet (base position 1).
				result[path][token.Pos(pos.Offset+1)] = true
			}
		}
	}
	return result
}

func TestRoleGrantObjects(t *testing.T) {
	const base = "github.com/nself-org/plugins/free/ci/internal/"
	for _, tc := range []struct {
		name, pkg    string
		method, want bool
	}{
		{"role type", "store", false, true},
		{"role method", "roles", true, true},
		{"lease method", "lease", true, false},
		{"store method", "store", true, false},
		{"unrelated type", "other", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := types.NewPackage(base+tc.pkg, tc.pkg)
			var obj types.Object = types.NewTypeName(token.NoPos, pkg, "Grant", nil)
			if tc.method {
				recv := types.NewVar(token.NoPos, pkg, "s", types.Typ[types.Int])
				obj = types.NewFunc(token.NoPos, pkg, "Grant", types.NewSignatureType(recv, nil, nil, nil, nil, false))
			}
			if got := isRoleGrant(obj); got != tc.want {
				t.Fatalf("isRoleGrant = %v, want %v", got, tc.want)
			}
		})
	}
}
