package main

// Purpose: the command list of Reference.md: every key the cmd package
// registers (register for v1.4, registerV15 for v1.5) merged with the
// commands block of plugin.json.
// Inputs:  the parsed cmd package; plugin.json.
// Outputs: Command rows sorted by key.
// Constraints: a key built in a range loop is expanded over the loop's
// literal list; the side-effect class comes from the table entry
// (SideEffect field, or the class field of the loop item), then from
// plugin.json. A key the scanner cannot resolve is an error, never skipped.

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"os"
	"sort"
	"strings"
)

// Command is one row of the command table.
type Command struct {
	Key        string
	Surface    string
	SideEffect string
	Summary    string
	Files      []string // cmd files that register the key
}

// manifestCommand is one subcommand of the plugin.json commands block.
type manifestCommand struct {
	Name        string            `json:"name"`
	Summary     string            `json:"summary"`
	SideEffect  string            `json:"side_effect"`
	Subcommands []manifestCommand `json:"subcommands"`
}

// manifest is the part of plugin.json the reference uses.
type manifest struct {
	Version  string            `json:"version"`
	Requires map[string]string `json:"requires"`
	Commands struct {
		Binary      string            `json:"binary"`
		Command     string            `json:"command"`
		SideEffect  string            `json:"side_effect"`
		Summary     string            `json:"summary"`
		Subcommands []manifestCommand `json:"subcommands"`
	} `json:"commands"`
}

// readManifest decodes plugin.json.
func readManifest(path string) (manifest, error) {
	var mf manifest
	b, err := os.ReadFile(path)
	if err != nil {
		return mf, err
	}
	return mf, json.Unmarshal(b, &mf)
}

// manifestIndex flattens the commands block by space-joined key.
func manifestIndex(subs []manifestCommand, prefix string, out map[string]manifestCommand) {
	for _, s := range subs {
		key := prefix + s.Name
		out[key] = s
		manifestIndex(s.Subcommands, key+" ", out)
	}
}

// commands returns every registered command of the cmd package.
func commands(m *module, mf manifest) ([]Command, error) {
	p := m.pkgs[modulePath+"/cmd"]
	if p == nil {
		return nil, fmt.Errorf("cmd package not found")
	}
	byKey := map[string]*Command{}
	var problems []string
	for _, f := range p.files {
		f := f
		w := walker{m: m, p: p, f: f}
		w.walk(f.ast, nil, nil, func(call *ast.CallExpr, env map[string][]string, item map[string]string) {
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || len(call.Args) != 2 {
				return
			}
			prefix, sub := subPrefix[fn.Name]
			if fn.Name != "register" && fn.Name != "registerV15" && !sub {
				return
			}
			keys := m.strs(p, f, call.Args[0], env)
			for i := range keys {
				keys[i] = prefix + keys[i]
			}
			if len(keys) == 0 {
				problems = append(problems, fmt.Sprintf("%s: %s: unresolved command key", f.name, fn.Name))
				return
			}
			for _, key := range keys {
				c := byKey[key]
				if c == nil {
					c = &Command{Key: key}
					byKey[key] = c
				}
				c.Files = addOnce(c.Files, f.name)
				if fn.Name == "register" {
					c.Surface = joinSurface(c.Surface, "v1.4")
					continue
				}
				if sub {
					c.Surface = joinSurface(c.Surface, "v1.5")
					continue
				}
				surface, effect := "v1.5", ""
				if cl, ok := call.Args[1].(*ast.CompositeLit); ok {
					fs := fields(cl)
					if isTrue(fs["Hidden"]) {
						surface = "v1.5 (hidden)"
					}
					if e, ok := fs["SideEffect"]; ok {
						effect, _ = m.str(p, f, e, env)
					}
				}
				if effect == "" {
					effect = item["class"]
				}
				c.Surface = joinSurface(c.Surface, surface)
				if effect != "" {
					c.SideEffect = effect
				}
			}
		})
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%v", problems)
	}
	idx := map[string]manifestCommand{}
	manifestIndex(mf.Commands.Subcommands, "", idx)
	out := make([]Command, 0, len(byKey))
	for _, c := range byKey {
		if mc, ok := idx[c.Key]; ok {
			c.Summary = mc.Summary
			if c.SideEffect == "" {
				c.SideEffect = mc.SideEffect
			}
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// joinSurface adds s to the comma list have, once.
func joinSurface(have, s string) string {
	switch {
	case have == "":
		return s
	case strings.Contains(", "+have+", ", ", "+s+", "):
		return have
	}
	return have + ", " + s
}

// subPrefix maps the helpers that register one sub-verb of a v1.5 verb to
// the verb they extend: registerRunsSub("approve", h) is "runs approve".
var subPrefix = map[string]string{"registerRunsSub": "runs ", "registerArtifactsSub": "artifacts "}

// addOnce appends s to list when it is not there.
func addOnce(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}
