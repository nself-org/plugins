package main

// Purpose: load the non-test Go source of the free/ci module and resolve
// string constants, so the generator reads commands and codes from code.
// Inputs:  the module directory.
// Outputs: one pkg per directory, with its files, imports and string consts.
// Constraints: vendor, testdata and _test.go files are skipped; only string
// constants built from literals, other constants and + are resolved.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// modulePath is the import path of free/ci.
const modulePath = "github.com/nself-org/plugins/free/ci"

// srcFile is one parsed file and its import table (local name to path).
type srcFile struct {
	name    string // base name of the source file
	ast     *ast.File
	imports map[string]string
}

// constDef is one package-level constant before resolution.
type constDef struct {
	file     *srcFile
	expr     ast.Expr
	typeName string
}

// pkg is one package of the module.
type pkg struct {
	path   string
	dir    string
	files  []*srcFile
	consts map[string]constDef
}

// module is every package of free/ci, by import path.
type module struct {
	pkgs     map[string]*pkg
	resolved map[string]string
	busy     map[string]bool
}

// loadModule parses every non-test Go file under root.
func loadModule(root string) (*module, error) {
	m := &module{pkgs: map[string]*pkg{}, resolved: map[string]string{}, busy: map[string]bool{}}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "vendor" || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		m.add(root, path, f)
		return nil
	})
	return m, err
}

// add records file f, read from path, under its package.
func (m *module) add(root, path string, f *ast.File) {
	dir := filepath.Dir(path)
	rel, _ := filepath.Rel(root, dir)
	ip := modulePath
	if rel != "." {
		ip += "/" + filepath.ToSlash(rel)
	}
	p := m.pkgs[ip]
	if p == nil {
		p = &pkg{path: ip, dir: dir, consts: map[string]constDef{}}
		m.pkgs[ip] = p
	}
	sf := &srcFile{name: filepath.Base(path), ast: f, imports: map[string]string{}}
	for _, im := range f.Imports {
		target, _ := strconv.Unquote(im.Path.Value)
		name := target[strings.LastIndex(target, "/")+1:]
		if im.Name != nil {
			name = im.Name.Name
		}
		sf.imports[name] = target
	}
	p.files = append(p.files, sf)
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			typeName := ""
			if id, ok := vs.Type.(*ast.Ident); ok {
				typeName = id.Name
			}
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					p.consts[n.Name] = constDef{file: sf, expr: vs.Values[i], typeName: typeName}
				}
			}
		}
	}
}

// sortedPkgs returns the packages in import-path order, so output is stable.
func (m *module) sortedPkgs() []*pkg {
	out := make([]*pkg, 0, len(m.pkgs))
	for _, p := range m.pkgs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// constValue resolves the string constant name of package path.
func (m *module) constValue(path, name string) (string, bool) {
	key := path + "." + name
	if v, ok := m.resolved[key]; ok {
		return v, true
	}
	p := m.pkgs[path]
	if p == nil || m.busy[key] {
		return "", false
	}
	def, ok := p.consts[name]
	if !ok {
		return "", false
	}
	m.busy[key] = true
	v, ok := m.str(p, def.file, def.expr, nil)
	delete(m.busy, key)
	if ok {
		m.resolved[key] = v
	}
	return v, ok
}

// str evaluates e to one string. env holds loop variables that have a
// single known value; see strs for several.
func (m *module) str(p *pkg, f *srcFile, e ast.Expr, env map[string][]string) (string, bool) {
	vals := m.strs(p, f, e, env)
	if len(vals) != 1 {
		return "", false
	}
	return vals[0], true
}

// strs evaluates e to every string it can take: a literal, a constant, a
// loop variable of env, or a + of those (the cross product).
func (m *module) strs(p *pkg, f *srcFile, e ast.Expr, env map[string][]string) []string {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return nil
		}
		s, err := strconv.Unquote(x.Value)
		if err != nil {
			return nil
		}
		return []string{s}
	case *ast.ParenExpr:
		return m.strs(p, f, x.X, env)
	case *ast.Ident:
		if vals, ok := env[x.Name]; ok {
			return vals
		}
		if v, ok := m.constValue(p.path, x.Name); ok {
			return []string{v}
		}
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && f != nil {
			if target, ok := f.imports[id.Name]; ok {
				if v, ok := m.constValue(target, x.Sel.Name); ok {
					return []string{v}
				}
			}
		}
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return nil
		}
		left, right := m.strs(p, f, x.X, env), m.strs(p, f, x.Y, env)
		var out []string
		for _, l := range left {
			for _, r := range right {
				out = append(out, l+r)
			}
		}
		return out
	}
	return nil
}

// fields maps the keyed fields of a composite literal to their expressions.
func fields(cl *ast.CompositeLit) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	for _, el := range cl.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok {
				out[id.Name] = kv.Value
			}
		}
	}
	return out
}

// isTrue reports whether e is the identifier true.
func isTrue(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "true"
}
