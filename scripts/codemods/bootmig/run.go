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
func bootMigrationsHealth() map[string]int {
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
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
	detail := fmt.Sprintf("pool=%s:%d health=%s", filepath.Base(s.file), s.line, map[bool]string{true: "wired", false: "manual"}[health != nil])
	if dry {
		return "converted " + detail, nil
	}
	src, _ := os.ReadFile(s.file)
	ind := indentOf(src, s.insert-1)
	byFile := map[string][]edit{s.file: {{s.insert, "\n" + ind + "mustBootMigrations(" + s.ctx + ", " + s.pool + ")"}}}
	if health != nil {
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
	file string
	edit edit
}

// healthEdit finds the map literal with a "status" key inside a health handler: a function whose name
// contains "ealth", or any call whose first argument is a string literal ending in "/health".
func healthEdit(files []string) *healthSite {
	for _, f := range files {
		src, _ := os.ReadFile(f)
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			continue
		}
		var found *healthSite
		var walk func(n ast.Node, inHealth bool)
		walk = func(n ast.Node, inHealth bool) {
			if n == nil || found != nil {
				return
			}
			ast.Inspect(n, func(c ast.Node) bool {
				if found != nil || c == n {
					return found == nil
				}
				switch v := c.(type) {
				case *ast.FuncDecl:
					walk(v.Body, inHealth || strings.Contains(v.Name.Name, "ealth"))
					return false
				case *ast.CallExpr:
					if len(v.Args) > 0 {
						if bl, ok := v.Args[0].(*ast.BasicLit); ok && strings.HasSuffix(strings.Trim(bl.Value, "\"`"), "/health") {
							for _, a := range v.Args[1:] {
								walk(a, true)
							}
						}
					}
				case *ast.CompositeLit:
					if inHealth && isStatusMap(v) {
						found = &healthSite{file: f, edit: litEdit(src, fset, v)}
						return false
					}
				}
				return true
			})
		}
		walk(af, false)
		if found != nil {
			return found
		}
	}
	return nil
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

func litEdit(src []byte, fset *token.FileSet, cl *ast.CompositeLit) edit {
	rb := fset.Position(cl.Rbrace).Offset
	if fset.Position(cl.Lbrace).Line == fset.Position(cl.Rbrace).Line {
		return edit{rb, `, "migrations": bootMigrationsHealth()`}
	}
	line := strings.LastIndexByte(string(src[:rb]), '\n') + 1
	return edit{line, indentOf(src, rb) + "\t\"migrations\": bootMigrationsHealth(),\n"}
}
