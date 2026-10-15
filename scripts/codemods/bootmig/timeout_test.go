package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the generated function against a context-aware stalled status reader.
// The isolated harness needs only the standard library, not a live database.
func TestHelperRequestTimeout(t *testing.T) {
	body := fmt.Sprintf(helperTmpl, "main", "example.com/x", "np_x")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "helper.go", body, 0)
	if err != nil {
		t.Fatal(err)
	}
	var helper string
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "bootMigrationsHealth" {
			helper = body[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset]
		}
	}
	dir := t.TempDir()
	src := `package main
import ("context"; "log"; "net/http"; "net/http/httptest"; "testing"; "time")
var bootPool any
func bootOptions() any { return nil }
type status struct { Applied, Expected int }
var migrate struct { Status func(context.Context, any, any) (status, error) }
` + helper + `
func TestBoundedStatus(t *testing.T) {
 for _, mode := range []string{"success", "cancel", "deadline"} {
  t.Run(mode, func(t *testing.T) {
   parent := context.WithValue(context.Background(), "request", mode)
   var cancel context.CancelFunc
   if mode == "cancel" { parent, cancel = context.WithCancel(parent); cancel() }
   if mode == "deadline" { parent, cancel = context.WithTimeout(parent, 20*time.Millisecond); defer cancel() }
   var used context.Context
   migrate.Status = func(ctx context.Context, _, _ any) (status, error) {
    used = ctx
    if ctx.Value("request") != mode { t.Fatal("lost request context") }
    deadline, ok := ctx.Deadline()
    if !ok || time.Until(deadline) > 5*time.Second { t.Fatal("missing five second bound") }
    if mode == "success" { return status{2, 2}, nil }
    select { case <-ctx.Done(): return status{}, ctx.Err(); case <-time.After(time.Second): t.Fatal("status did not stop"); return status{}, nil }
   }
   w := httptest.NewRecorder()
   got := bootMigrationsHealth(parent, w)
   if used.Err() == nil { t.Error("helper did not release its context") }
   if mode == "success" {
    if w.Code != 200 || got["applied"] != 2 || got["expected"] != 2 { t.Fatalf("success: %d %v", w.Code, got) }
   } else if w.Code != 503 || got["applied"] != -1 || got["expected"] != -1 { t.Fatalf("failure: %d %v", w.Code, got) }
  })
 }
}
`
	if err := os.WriteFile(filepath.Join(dir, "helper_test.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-timeout=10s", "helper_test.go")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated helper: %v\n%s", err, out)
	}
}
