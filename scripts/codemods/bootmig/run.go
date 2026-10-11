package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const helperTmpl = `// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Written by
// scripts/codemods/boot-migrations.sh; edit by hand if the plugin needs tolerant mode.
package %s

import (
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"%s/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "%s", FS: migrations.FS}
}

// mustBootMigrations applies the plugin's own migrations before it serves; a failure is fatal.
func mustBootMigrations(ctx context.Context, pool *pgxpool.Pool) {
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		log.Fatalf("migrations: %%v", err)
	}
	log.Printf("migrations: applied %%d", len(res.Applied))
}

// bootMigrationsHealth is the "migrations" member of /health: applied and expected counts.
// On a Status error it logs, answers 503 (the cli retries any non-200; a 200 with -1 counts is E119,
// a contract violation) and returns -1 counts for humans. Call it before the handler writes a header.
func bootMigrationsHealth(w http.ResponseWriter) map[string]int {
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %%v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}
`

const embedSrc = `// Package migrations embeds the plugin's SQL migration files for sdk/go/migrate.
package migrations

import "embed"

// FS holds migrations/*.sql.
//
//go:embed *.sql
var FS embed.FS
`

func run(dir, schema string, dry bool) (string, error) {
	gm, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("no go.mod in %s", dir)
	}
	m := moduleRE.FindSubmatch(gm)
	if m == nil {
		return "", fmt.Errorf("go.mod has no module line")
	}
	files := goFiles(dir)
	if anyContains(files, "mustBootMigrations(", "migrate.Apply(") {
		return "noop", nil
	}
	sites, err := findPools(files)
	if err != nil {
		return "", err
	}
	if len(sites) == 0 {
		return "manual: no pgxpool.New call (pool pattern not found)", nil
	}
	if len(sites) > 1 {
		return fmt.Sprintf("manual: %d pgxpool.New calls; wire migrate.Apply by hand", len(sites)), nil
	}
	s := sites[0]
	health := healthEdit(files)
	wired := health != nil && !health.manual
	detail := fmt.Sprintf("pool=%s:%d health=%s", filepath.Base(s.file), s.line, map[bool]string{true: "wired", false: "manual"}[wired])
	if dry {
		return "converted " + detail, nil
	}
	src, _ := os.ReadFile(s.file)
	ind := indentOf(src, s.insert-1)
	byFile := map[string][]edit{s.file: {{s.insert, "\n" + ind + "mustBootMigrations(" + s.ctx + ", " + s.pool + ")"}}}
	if wired {
		byFile[health.file] = append(byFile[health.file], health.edit)
	}
	for f, es := range byFile {
		if err := applyEdits(f, es); err != nil {
			return "", err
		}
	}
	helper := filepath.Join(filepath.Dir(s.file), "boot_migrations.go")
	body := fmt.Sprintf(helperTmpl, s.pkg, string(m[1]), schema)
	if err := os.WriteFile(helper, []byte(body), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "migrations", "embed.go"), []byte(embedSrc), 0o644); err != nil {
		return "", err
	}
	return "converted " + detail, nil
}

type healthSite struct {
	file   string
	edit   edit
	manual bool // the health literal was found but cannot be wired safely
}

// fnScope is the function (declaration or literal) enclosing a node: its type and body.
type fnScope struct {
	typ  *ast.FuncType
	body *ast.BlockStmt
}

// healthEdit finds the map literal with a "status" key inside a health handler: a function whose name
// contains "ealth", or any call whose first argument is a string literal ending in "/health". The literal
// gets `"migrations": bootMigrationsHealth(<w>)`, <w> being the enclosing function's http.ResponseWriter
// parameter. With no such parameter, or a <w>.WriteHeader call before the literal, nothing is wired
// (the helper's 503 would be lost or superfluous) and the site is left for hand wiring.
func healthEdit(files []string) *healthSite {
	for _, f := range files {
		src, _ := os.ReadFile(f)
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			continue
		}
		var found *healthSite
		var walk func(n ast.Node, inHealth bool, fn fnScope)
		walk = func(n ast.Node, inHealth bool, fn fnScope) {
			if n == nil || found != nil {
				return
			}
			if fl, ok := n.(*ast.FuncLit); ok {
				fn = fnScope{fl.Type, fl.Body}
			}
			ast.Inspect(n, func(c ast.Node) bool {
				if found != nil || c == n {
					return found == nil
				}
				switch v := c.(type) {
				case *ast.FuncDecl:
					walk(v.Body, inHealth || strings.Contains(v.Name.Name, "ealth"), fnScope{v.Type, v.Body})
					return false
				case *ast.FuncLit:
					walk(v, inHealth, fn)
					return false
				case *ast.CallExpr:
					if len(v.Args) > 0 {
						if bl, ok := v.Args[0].(*ast.BasicLit); ok && strings.HasSuffix(strings.Trim(bl.Value, "\"`"), "/health") {
							for _, a := range v.Args[1:] {
								walk(a, true, fn)
							}
						}
					}
				case *ast.CompositeLit:
					if inHealth && isStatusMap(v) {
						w := writerParam(fn.typ)
						if w == "" || writesHeaderBefore(fn.body, w, v.Pos()) {
							found = &healthSite{manual: true}
						} else {
							found = &healthSite{file: f, edit: litEdit(src, fset, v, w)}
						}
						return false
					}
				}
				return true
			})
		}
		walk(af, false, fnScope{})
		if found != nil {
			return found
		}
	}
	return nil
}

// writerParam returns the name of the http.ResponseWriter parameter of ft, or "" when there is none.
func writerParam(ft *ast.FuncType) string {
	if ft == nil || ft.Params == nil {
		return ""
	}
	for _, p := range ft.Params.List {
		sel, ok := p.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ResponseWriter" || len(p.Names) == 0 || p.Names[0].Name == "_" {
			continue
		}
		return p.Names[0].Name
	}
	return ""
}

// writesHeaderBefore reports whether body calls <w>.WriteHeader(...) at a position before pos.
func writesHeaderBefore(body *ast.BlockStmt, w string, pos token.Pos) bool {
	if body == nil {
		return false
	}
	hit := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && call.Pos() < pos {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "WriteHeader" {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == w {
					hit = true
				}
			}
		}
		return !hit
	})
	return hit
}
func isStatusMap(cl *ast.CompositeLit) bool {
	if _, ok := cl.Type.(*ast.MapType); !ok {
		return false
	}
	has := false
	for _, e := range cl.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if bl, ok := kv.Key.(*ast.BasicLit); ok {
				k := strings.Trim(bl.Value, "\"`")
				if k == "migrations" {
					return false
				}
				has = has || k == "status"
			}
		}
	}
	return has
}

func litEdit(src []byte, fset *token.FileSet, cl *ast.CompositeLit, w string) edit {
	rb := fset.Position(cl.Rbrace).Offset
	if fset.Position(cl.Lbrace).Line == fset.Position(cl.Rbrace).Line {
		return edit{rb, `, "migrations": bootMigrationsHealth(` + w + `)`}
	}
	line := strings.LastIndexByte(string(src[:rb]), '\n') + 1
	return edit{line, indentOf(src, rb) + "\t\"migrations\": bootMigrationsHealth(" + w + "),\n"}
}
