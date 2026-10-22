package dispatch

// Resolve only the two unrelated role-management symbols named Grant. Lease
// engine methods and unresolved references still go through the strict scanner.
import (
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func roleGrantPositions(t *testing.T, root string) map[string]map[int]bool {
	t.Helper()
	out := map[string]map[int]bool{}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return out
	}
	pkgs, err := packages.Load(&packages.Config{Dir: root, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports}, "./cmd", "./internal/roles", "./internal/store")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) != 0 {
			t.Fatalf("resolve role Grant symbols: %v", pkg.Errors)
		}
		mark := func(pos types.Object) bool {
			if pos == nil || pos.Pkg() == nil || pos.Name() != "Grant" {
				return false
			}
			switch pos.(type) {
			case *types.TypeName:
				return pos.Pkg().Path() == "github.com/nself-org/plugins/free/ci/internal/store"
			case *types.Func:
				return pos.Pkg().Path() == "github.com/nself-org/plugins/free/ci/internal/roles"
			}
			return false
		}
		record := func(pos int, name string) {
			if out[name] == nil {
				out[name] = map[int]bool{}
			}
			out[name][pos] = true
		}
		for id, obj := range pkg.TypesInfo.Uses {
			if mark(obj) {
				p := pkg.Fset.Position(id.Pos())
				record(p.Offset, p.Filename)
			}
		}
		for id, obj := range pkg.TypesInfo.Defs {
			if mark(obj) {
				p := pkg.Fset.Position(id.Pos())
				record(p.Offset, p.Filename)
			}
		}
	}
	return out
}
