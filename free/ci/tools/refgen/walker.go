package main

// Purpose: visit the calls of a file with the values of its loop variables
// known, so `registerV15("nodes "+name, …)` inside a range over a literal
// list expands to one key per list item.
// Inputs:  a parsed file of a package.
// Outputs: one visit per call per loop item, with that item's fields.
// Constraints: only a range over a composite literal, or over a package var
// initialised with one, is expanded; struct items need keyed fields or a
// struct type in the literal so positional fields can be named.

import (
	"go/ast"
	"go/token"
)

// walker carries the package context the evaluator needs.
type walker struct {
	m *module
	p *pkg
	f *srcFile
}

// visitFunc receives a call, the loop variables in scope and the fields of
// the innermost struct loop item.
type visitFunc func(call *ast.CallExpr, env map[string][]string, item map[string]string)

// item is one element of a ranged literal list: a string or a struct.
type item struct {
	str    string
	fields map[string]string
}

// walk visits every call under n. Each range over a known list walks its
// body once per item.
func (w walker) walk(n ast.Node, env map[string][]string, cur map[string]string, visit visitFunc) {
	ast.Inspect(n, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.CallExpr:
			visit(x, env, cur)
		case *ast.RangeStmt:
			v, ok := x.Value.(*ast.Ident)
			if !ok {
				return true
			}
			list := w.rangeItems(x.X)
			if list == nil {
				return true
			}
			for _, it := range list {
				inner := copyEnv(env)
				fieldsOf := cur
				if it.fields != nil {
					fieldsOf = it.fields
				} else {
					inner[v.Name] = []string{it.str}
				}
				bindCopies(x.Body, v.Name, it, inner)
				w.walk(x.Body, inner, fieldsOf, visit)
			}
			return false
		}
		return true
	})
}

// bindCopies binds the := copies of the loop variable, or of one of its
// fields, at the top level of body.
func bindCopies(body *ast.BlockStmt, loopVar string, it item, env map[string][]string) {
	for _, st := range body.List {
		as, ok := st.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			continue
		}
		lhs, ok := as.Lhs[0].(*ast.Ident)
		if !ok {
			continue
		}
		switch r := as.Rhs[0].(type) {
		case *ast.Ident:
			if r.Name == loopVar && it.fields == nil {
				env[lhs.Name] = []string{it.str}
			}
		case *ast.SelectorExpr:
			if id, ok := r.X.(*ast.Ident); ok && id.Name == loopVar {
				if val, ok := it.fields[r.Sel.Name]; ok {
					env[lhs.Name] = []string{val}
				}
			}
		}
	}
}

// rangeItems resolves the ranged expression to its literal items, or nil.
func (w walker) rangeItems(e ast.Expr) []item {
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		id, isIdent := e.(*ast.Ident)
		if !isIdent {
			return nil
		}
		cl = w.packageVar(id.Name)
		if cl == nil {
			return nil
		}
	}
	var names []string
	if at, ok := cl.Type.(*ast.ArrayType); ok {
		if st, ok := at.Elt.(*ast.StructType); ok {
			for _, fl := range st.Fields.List {
				for _, n := range fl.Names {
					names = append(names, n.Name)
				}
			}
		}
	}
	var out []item
	for _, el := range cl.Elts {
		if sub, ok := el.(*ast.CompositeLit); ok {
			fs := map[string]string{}
			for i, x := range sub.Elts {
				if kv, ok := x.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok {
						if v, ok := w.m.str(w.p, w.f, kv.Value, nil); ok {
							fs[k.Name] = v
						}
					}
				} else if i < len(names) {
					if v, ok := w.m.str(w.p, w.f, x, nil); ok {
						fs[names[i]] = v
					}
				}
			}
			out = append(out, item{fields: fs})
			continue
		}
		s, ok := w.m.str(w.p, w.f, el, nil)
		if !ok {
			return nil
		}
		out = append(out, item{str: s})
	}
	return out
}

// packageVar finds the composite literal a package-level var is set to.
func (w walker) packageVar(name string) *ast.CompositeLit {
	for _, f := range w.p.files {
		for _, decl := range f.ast.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if n.Name == name && i < len(vs.Values) {
						cl, _ := vs.Values[i].(*ast.CompositeLit)
						return cl
					}
				}
			}
		}
	}
	return nil
}

// copyEnv returns a shallow copy of env.
func copyEnv(env map[string][]string) map[string][]string {
	out := map[string][]string{}
	for k, v := range env {
		out[k] = v
	}
	return out
}
