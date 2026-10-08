package architecture

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const module = "github.com/nself-org/plugins/free/ci/"

type layerMap struct {
	Version int                 `yaml:"version"`
	Layers  map[string][]string `yaml:"layers"`
}
type goPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
}

func readLayers(t *testing.T) layerMap {
	t.Helper()
	b, err := os.ReadFile("layers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var m layerMap
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != 1 {
		t.Fatalf("layers version = %d", m.Version)
	}
	return m
}

func kindOf(m layerMap, pkg string) string {
	for kind, names := range m.Layers {
		for _, name := range names {
			if pkg == name {
				return kind
			}
		}
	}
	return ""
}

// checkImport applies planned package boundaries to one source-file import.
func checkImport(m layerMap, pkg, file, imported string) error {
	kind := kindOf(m, pkg)
	if kind == "" {
		return fmt.Errorf("package %s has no layers row", pkg)
	}
	local := strings.TrimPrefix(imported, module)
	isLocal := imported != local
	if imported == "os/exec" && pkg != "internal/exec" && pkg != "internal/legacy" {
		// These frozen pre-fabric sites are tracked by the single-exec-site test.
		for _, old := range []struct{ pkg, file string }{{"internal/forgejo", "forgejo.go"}, {"internal/nodes/provision", "exec.go"}, {"internal/serve", "serve_job.go"}, {"internal/serve", "serve_job_report.go"}} {
			if pkg == old.pkg && file == old.file && kind == "preexisting" {
				return nil
			}
		}
		return fmt.Errorf("%s imports os/exec", pkg)
	}
	if (imported == "database/sql" || imported == "modernc.org/sqlite") && pkg != "internal/store" {
		return fmt.Errorf("%s imports SQL outside store", pkg)
	}
	if (kind == "pure" || kind == "kernel") && (imported == "os" || imported == "net" || strings.HasPrefix(imported, "net/")) {
		return fmt.Errorf("%s is pure but imports %s", pkg, imported)
	}
	if !isLocal {
		return nil
	}
	if local == "cmd" || strings.HasPrefix(local, "cmd/") {
		return fmt.Errorf("%s imports cmd", pkg)
	}
	if local == "internal/legacy" && pkg != "cmd" {
		return fmt.Errorf("%s imports legacy", pkg)
	}
	if kind == "preexisting" {
		return nil // existing code retains its other import edges
	}
	if pkg == "internal/legacy" && strings.HasPrefix(local, "internal/") && local != "internal/legacy" {
		return fmt.Errorf("legacy imports new package %s", local)
	}
	if (kind == "kernel") && strings.HasPrefix(local, "internal/") {
		return fmt.Errorf("kernel %s imports %s", pkg, local)
	}
	if (pkg == "internal/plan" || pkg == "internal/policy") && local == "internal/plan" && pkg == "internal/policy" {
		return fmt.Errorf("policy imports plan")
	}
	if pkg == "internal/run" && local == "internal/dispatch" && file != "dispatch_hook.go" {
		return fmt.Errorf("run imports dispatch outside hook")
	}
	if pkg == "internal/run" && strings.HasPrefix(local, "internal/providers") && file != "trust_hook.go" {
		return fmt.Errorf("run imports providers outside hook")
	}
	if pkg == "internal/dispatch" && strings.HasPrefix(local, "internal/") {
		allowed := []string{"world", "sched", "store", "lease", "transport", "exec", "model"}
		ok := false
		for _, a := range allowed {
			if local == "internal/"+a || strings.HasPrefix(local, "internal/"+a+"/") {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("dispatch imports %s", local)
		}
	}
	if pkg == "internal/store" && strings.HasPrefix(local, "internal/providers") {
		return fmt.Errorf("store imports providers")
	}
	if strings.HasPrefix(pkg, "internal/providers") || strings.HasPrefix(pkg, "internal/triggers") {
		for _, banned := range []string{"internal/run", "internal/dispatch", "cmd"} {
			if local == banned {
				return fmt.Errorf("%s imports %s", pkg, local)
			}
		}
	}
	if strings.HasPrefix(local, "internal/providers") || strings.HasPrefix(local, "internal/triggers") {
		for _, banned := range []string{"internal/model", "internal/compatcheck", "internal/plan", "internal/detect", "internal/diagnose", "internal/sched", "internal/trust", "internal/invariants", "internal/lease", "internal/store", "internal/world", "internal/dispatch"} {
			if pkg == banned || strings.HasPrefix(pkg, banned+"/") {
				return fmt.Errorf("%s imports %s", pkg, local)
			}
		}
	}
	for _, pure := range []string{"internal/blob/cas", "internal/blob/transfer", "internal/cache/entries", "internal/release/manifest", "internal/release/verify", "internal/provenance", "internal/cache/locality"} {
		if pkg == pure && (imported == "os/exec" || imported == "database/sql" || imported == "net" || strings.HasPrefix(imported, "net/")) {
			return fmt.Errorf("%s imports %s", pkg, imported)
		}
	}
	if pkg == "tools/manifestgen" && isLocal && local != "internal/model" {
		return fmt.Errorf("manifestgen imports %s", local)
	}
	return nil
}

func TestLayers(t *testing.T) {
	m := readLayers(t)
	cmd := exec.Command("go", "list", "-deps", "-json", "./...")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=vendor", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	checked := 0
	for dec.More() {
		var p goPackage
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(p.ImportPath, module) {
			continue
		}
		pkg := strings.TrimPrefix(p.ImportPath, module)
		if strings.HasPrefix(pkg, "internal/") && kindOf(m, pkg) == "" {
			t.Errorf("package %s has no layers row", pkg)
		}
		for _, file := range p.GoFiles {
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.Dir, file), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if err := checkImport(m, pkg, file, path); err != nil {
					t.Error(err)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no imports inspected")
	}
	for _, c := range []struct{ pkg, file, imp string }{
		{"internal/plan", "x.go", "os"}, {"internal/model", "x.go", "net"},
		{"internal/cache/entries", "x.go", "os/exec"}, {"internal/store", "x.go", module + "internal/providers"},
		{"internal/run", "x.go", module + "internal/dispatch"}, {"internal/policy", "x.go", module + "internal/plan"},
		{"internal/newpackage", "x.go", "os/exec"},
	} {
		if checkImport(m, c.pkg, c.file, c.imp) == nil {
			t.Errorf("fixture %s -> %s passed", c.pkg, c.imp)
		}
	}
}

func TestRequiredRows(t *testing.T) {
	m := readLayers(t)
	f, err := os.Open("testdata/required-rows.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	scan := bufio.NewScanner(f)
	count := 0
	for scan.Scan() {
		row := strings.TrimSpace(scan.Text())
		if row == "" {
			continue
		}
		count++
		if kindOf(m, row) == "" {
			t.Errorf("required row %s missing", row)
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 100 {
		t.Fatalf("required rows unexpectedly short: %d", count)
	}
}
