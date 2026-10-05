package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type edit struct {
	off  int
	text string
}

type site struct {
	file   string
	pkg    string
	line   int
	insert int
	ctx    string
	pool   string
}

var moduleRE = regexp.MustCompile(`(?m)^module\s+(\S+)`)

// goFiles lists the plugin's own non-test Go files (not vendor/, not third_party/).
func goFiles(dir string) []string {
	var out []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && (info.Name() == "vendor" || info.Name() == "third_party" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func indentOf(src []byte, off int) string {
	start := bytes.LastIndexByte(src[:off], '\n') + 1
	end := start
	for end < len(src) && (src[end] == '\t' || src[end] == ' ') {
		end++
	}
	return string(src[start:end])
}

func isErrCheck(s ast.Stmt) bool {
	ifs, ok := s.(*ast.IfStmt)
	if !ok {
		return false
	}
	be, ok := ifs.Cond.(*ast.BinaryExpr)
	if !ok || be.Op != token.NEQ {
		return false
	}
	id, ok := be.X.(*ast.Ident)
	return ok && id.Name == "err"
}

// findPools returns every `pool, err := pgxpool.New(ctx, ...)` site.
func findPools(files []string) ([]site, error) {
	var sites []site
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			blk, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i, st := range blk.List {
				as, ok := st.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
					continue
				}
				call, ok := as.Rhs[0].(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					continue
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "New" && sel.Sel.Name != "NewWithConfig") {
					continue
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "pgxpool" {
					continue
				}
				id, ok := as.Lhs[0].(*ast.Ident)
				if !ok || id.Name == "_" {
					continue
				}
				end := as.End()
				if i+1 < len(blk.List) && isErrCheck(blk.List[i+1]) {
					end = blk.List[i+1].End()
				}
				a, b := fset.Position(call.Args[0].Pos()).Offset, fset.Position(call.Args[0].End()).Offset
				sites = append(sites, site{file: f, pkg: af.Name.Name, line: fset.Position(as.Pos()).Line,
					insert: fset.Position(end).Offset, ctx: string(src[a:b]), pool: id.Name})
			}
			return true
		})
	}
	return sites, nil
}

// applyEdits inserts the edits (descending offsets), gofmts the result and writes the file.
func applyEdits(file string, edits []edit) error {
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].off > edits[j].off })
	for _, e := range edits {
		src = append(src[:e.off], append([]byte(e.text), src[e.off:]...)...)
	}
	out, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("format %s: %w", file, err)
	}
	return os.WriteFile(file, out, 0o644)
}

func anyContains(files []string, needles ...string) bool {
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, n := range needles {
			if bytes.Contains(b, []byte(n)) {
				return true
			}
		}
	}
	return false
}
