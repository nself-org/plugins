package main

// Purpose: the registries of Reference.md read from source: error codes
// (every model.Code literal), doctor check ids (every CI-D… string) and
// reason codes (every string constant of a type named *Reason).
// Inputs:  the parsed module.
// Outputs: sorted rows; an error for a code literal whose ID cannot be read.
// Constraints: the scan covers packages cmd does not link yet, so a code is
// documented the day it is registered.

import (
	"fmt"
	"go/ast"
	"regexp"
	"sort"
	"strings"
)

// modelPath is the import path of the package that declares Code.
const modelPath = modulePath + "/internal/model"

// ErrorCode is one registered error code.
type ErrorCode struct {
	ID, Class, Summary, Fix, Package string
}

// DoctorCheck is one doctor check id and the name it is reported under.
type DoctorCheck struct {
	ID, Name, Package string
}

// ReasonCode is one reason constant and the type that holds it.
type ReasonCode struct {
	Value, Type, Package string
}

// doctorID matches a doctor check id.
var doctorID = regexp.MustCompile(`^CI-D[A-Z]*[0-9]+$`)

// shortPkg trims the module path from an import path.
func shortPkg(path string) string {
	if path == modulePath {
		return "."
	}
	return strings.TrimPrefix(path, modulePath+"/")
}

// isCodeType reports whether t names model.Code from file f of package p.
func isCodeType(p *pkg, f *srcFile, t ast.Expr) bool {
	switch x := t.(type) {
	case *ast.Ident:
		return p.path == modelPath && x.Name == "Code"
	case *ast.SelectorExpr:
		id, ok := x.X.(*ast.Ident)
		return ok && x.Sel.Name == "Code" && f.imports[id.Name] == modelPath
	}
	return false
}

// errorCodes returns every model.Code literal of the module.
func errorCodes(m *module) ([]ErrorCode, error) {
	var out []ErrorCode
	var problems []string
	for _, p := range m.sortedPkgs() {
		for _, f := range p.files {
			ast.Inspect(f.ast, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				var lits []*ast.CompositeLit
				if isCodeType(p, f, cl.Type) {
					lits = append(lits, cl)
				} else if at, ok := cl.Type.(*ast.ArrayType); ok && isCodeType(p, f, at.Elt) {
					for _, el := range cl.Elts {
						if sub, ok := el.(*ast.CompositeLit); ok && sub.Type == nil {
							lits = append(lits, sub)
						}
					}
				}
				for _, lit := range lits {
					c, ok := codeOf(m, p, f, lit)
					if !ok {
						problems = append(problems, shortPkg(p.path)+": unresolved code literal")
						continue
					}
					out = append(out, c)
				}
				return true
			})
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// codeOf reads one Code literal, keyed or positional. Only the ID must
// resolve; a text field built at run time is left empty.
func codeOf(m *module, p *pkg, f *srcFile, cl *ast.CompositeLit) (ErrorCode, bool) {
	c := ErrorCode{Package: shortPkg(p.path)}
	dst := map[string]*string{"ID": &c.ID, "Class": &c.Class, "Summary": &c.Summary, "Fix": &c.Fix}
	order := []string{"ID", "Class", "Summary", "Fix"}
	for i, el := range cl.Elts {
		name, val := "", el
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if k, ok := kv.Key.(*ast.Ident); ok {
				name, val = k.Name, kv.Value
			}
		} else if i < len(order) {
			name = order[i]
		}
		if d, ok := dst[name]; ok {
			*d, _ = m.str(p, f, val, nil)
		}
	}
	return c, c.ID != ""
}

// doctorChecks returns every doctor check id of the module, with the Name of
// the literal that sets it when there is one.
func doctorChecks(m *module) []DoctorCheck {
	seen := map[string]DoctorCheck{}
	add := func(id, name, pkgPath string) {
		have, ok := seen[id]
		if !ok || (have.Name == "" && name != "") {
			seen[id] = DoctorCheck{ID: id, Name: name, Package: shortPkg(pkgPath)}
		}
	}
	for _, p := range m.sortedPkgs() {
		for name := range p.consts {
			if v, ok := m.constValue(p.path, name); ok && doctorID.MatchString(v) {
				add(v, "", p.path)
			}
		}
		for _, f := range p.files {
			ast.Inspect(f.ast, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CompositeLit:
					fs := fields(x)
					if e, ok := fs["ID"]; ok {
						if id, ok := m.str(p, f, e, nil); ok && doctorID.MatchString(id) {
							label, _ := m.str(p, f, fs["Name"], nil)
							add(id, label, p.path)
						}
					}
				case *ast.BasicLit:
					if v, ok := m.str(p, f, x, nil); ok && doctorID.MatchString(v) {
						add(v, "", p.path)
					}
				}
				return true
			})
		}
	}
	out := make([]DoctorCheck, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// reasonCodes returns every string constant whose declared type name ends
// in Reason.
func reasonCodes(m *module) []ReasonCode {
	var out []ReasonCode
	for _, p := range m.sortedPkgs() {
		for name, def := range p.consts {
			if !strings.HasSuffix(def.typeName, "Reason") {
				continue
			}
			if v, ok := m.constValue(p.path, name); ok {
				out = append(out, ReasonCode{Value: v, Type: def.typeName, Package: shortPkg(p.path)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Value < b.Value
	})
	return out
}
