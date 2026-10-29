package main

// Purpose: the flag table of Reference.md: every flag name the cmd package
// defines on a FlagSet, with the commands it belongs to.
// Inputs:  the parsed cmd package and the command rows.
// Outputs: Markdown table rows sorted by flag.
// Constraints: a flag belongs to the commands registered in the same file;
// a file that registers none (providers_authorize.go) falls back to the
// commands whose first word is the file name up to its first underscore.
// A flag name that is not a string literal or constant is an error.

import (
	"fmt"
	"go/ast"
	"sort"
	"strings"
)

// flagDefs maps a FlagSet method to the index of its name argument.
var flagDefs = map[string]int{
	"Bool": 0, "String": 0, "Int": 0, "Int64": 0, "Uint": 0, "Float64": 0, "Duration": 0,
	"Func": 0, "BoolFunc": 0,
	"BoolVar": 1, "StringVar": 1, "IntVar": 1, "Int64Var": 1, "UintVar": 1, "Float64Var": 1, "DurationVar": 1, "Var": 1,
}

// FlagRow is one flag and the commands that take it.
type FlagRow struct {
	Name     string
	Commands []string
}

// flagPackages lists the packages outside cmd that define flags of a verb,
// with the verb that owns them: `nodes add` builds its FlagSet in
// internal/nodes/lifecycle, and serve flags are added by packages through
// coordinator.RegisterFlag. TestNoFlagDefinedOutsideScannedPackages fails
// when another package defines a flag, so a flag is never silently omitted.
var flagPackages = map[string]string{
	modulePath + "/internal/nodes/lifecycle":   "nodes",
	modulePath + "/internal/coordinator/drain": "serve",
	modulePath + "/internal/config":            "config",
	modulePath + "/internal/doctor":            "doctor",
	modulePath + "/internal/discovery":         "nodes discover",
}

// listenerVerb owns every flag registered with coordinator.RegisterListener:
// the listener flags are serve flags whichever file registers them.
const listenerVerb = "serve"

// fileFlagNames returns the flag names a file defines on a FlagSet.
func fileFlagNames(m *module, p *pkg, f *srcFile) ([]string, error) {
	var names []string
	var bad error
	ast.Inspect(f.ast, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		i, ok := flagDefs[sel.Sel.Name]
		if !ok || len(call.Args) <= i || len(call.Args) < 2 {
			return true
		}
		name, ok := m.str(p, f, call.Args[i], nil)
		if !ok {
			if _, lit := call.Args[i].(*ast.BasicLit); lit {
				bad = fmt.Errorf("%s: flag name not resolvable", f.name)
			}
			return true
		}
		names = append(names, name)
		return true
	})
	return names, bad
}

// listenerFlagNames returns the flag names a file registers through
// RegisterListener(name, flag, ...), expanding a range over a literal list
// (`for _, l := range []struct{...}{...} { RegisterListener(l.name, l.flag, ...) }`).
// A flag name it cannot resolve is an error: the table would silently lose a flag.
func listenerFlagNames(m *module, p *pkg, f *srcFile) ([]string, error) {
	var names []string
	var bad error
	w := walker{m: m, p: p, f: f}
	w.walk(f.ast, nil, nil, func(call *ast.CallExpr, env map[string][]string, it map[string]string) {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "RegisterListener" || len(call.Args) < 2 {
			return
		}
		got := m.strs(p, f, call.Args[1], env)
		if s, ok := call.Args[1].(*ast.SelectorExpr); ok && len(got) == 0 {
			if v, ok := it[s.Sel.Name]; ok {
				got = []string{v}
			}
		}
		if len(got) == 0 {
			bad = fmt.Errorf("%s: RegisterListener flag name not resolvable", f.name)
			return
		}
		names = append(names, got...)
	})
	return names, bad
}

// ownersOfVerb returns the keys of the commands that are verb or start with it.
func ownersOfVerb(cmds []Command, verb string) []string {
	var out []string
	for _, c := range cmds {
		if c.Key == verb || strings.HasPrefix(c.Key, verb+" ") {
			out = append(out, c.Key)
		}
	}
	return out
}

// flagRows returns the flags of the cmd package and of flagPackages, sorted by name.
func flagRows(m *module, cmds []Command) ([]FlagRow, error) {
	p := m.pkgs[modulePath+"/cmd"]
	if p == nil {
		return nil, fmt.Errorf("cmd package not found")
	}
	byFile := map[string][]string{}
	for _, c := range cmds {
		for _, f := range c.Files {
			byFile[f] = append(byFile[f], c.Key)
		}
	}
	got := map[string]map[string]bool{}
	add := func(name string, owners []string) {
		if got[name] == nil {
			got[name] = map[string]bool{}
		}
		for _, o := range owners {
			got[name][o] = true
		}
	}
	serve := ownersOfVerb(cmds, listenerVerb)
	for _, f := range p.files {
		owners := byFile[f.name]
		if len(owners) == 0 {
			stem := strings.TrimSuffix(f.name, ".go")
			if i := strings.Index(stem, "_"); i > 0 {
				stem = stem[:i]
			}
			for _, c := range cmds {
				if c.Key == stem || strings.HasPrefix(c.Key, stem+" ") {
					owners = append(owners, c.Key)
				}
			}
		}
		names, err := fileFlagNames(m, p, f)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			add(n, owners)
		}
		if names, err = listenerFlagNames(m, p, f); err != nil {
			return nil, err
		}
		for _, n := range names {
			add(n, serve)
		}
	}
	for path, verb := range flagPackages {
		ep := m.pkgs[path]
		if ep == nil {
			return nil, fmt.Errorf("flag package %s not found", path)
		}
		owners := ownersOfVerb(cmds, verb)
		for _, f := range ep.files {
			names, err := fileFlagNames(m, ep, f)
			if err != nil {
				return nil, err
			}
			for _, n := range names {
				add(n, owners)
			}
		}
	}
	var out []FlagRow
	for n, set := range got {
		r := FlagRow{Name: n}
		for k := range set {
			r.Commands = append(r.Commands, k)
		}
		r.Commands = collapse(r.Commands)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// collapse shortens a command list: when a flag belongs to more than one
// command of a verb, the verb is written once as `verb *`, because the flag
// is shared by the file, not proven for every sub-verb.
func collapse(keys []string) []string {
	count := map[string]int{}
	for _, k := range keys {
		count[strings.Fields(k)[0]]++
	}
	seen := map[string]bool{}
	var out []string
	for _, k := range keys {
		v := strings.Fields(k)[0]
		e := k
		if count[v] > 1 {
			e = v + " *"
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out
}
