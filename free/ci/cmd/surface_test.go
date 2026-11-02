package main

// Purpose: ties free/ci/surface/commands.yaml (the authored machine surface, P7-CI-62, CI D31) to the code: the key set of
//          cmd/subcommands.go, the side-effect class of each key, each Flags constructor and plugin.json.
// Constraints: no network, no process started; reads the spec and the manifest from the module root.

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type specFlag struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	CLIOnly bool   `yaml:"cli_only"`
}

type specRow struct {
	Path       string `yaml:"path"`
	SideEffect string `yaml:"side_effect"`
	JSON       string `yaml:"json"`
	Surface    string `yaml:"surface"`
	Confirm    *struct {
		Flags []string `yaml:"flags"`
	} `yaml:"confirm"`
	Args []struct {
		Name     string `yaml:"name"`
		Variadic bool   `yaml:"variadic"`
	} `yaml:"args"`
	Flags []specFlag `yaml:"flags"`
}

type specFile struct {
	Root     specRow   `yaml:"root"`
	Commands []specRow `yaml:"commands"`
}

func loadSpec(t *testing.T) specFile {
	t.Helper()
	raw, err := os.ReadFile("../surface/commands.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var s specFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// tableKeys is every key a machine surface row must exist for.
func tableKeys() []string {
	set := map[string]bool{}
	for k := range subcommands {
		set[k] = true
	}
	for k := range v15Subcommands {
		set[k] = true
	}
	for k := range runsSubs {
		set["runs "+k] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// flagType maps a registered flag value to the manifest type name.
func flagType(f *flag.Flag) string {
	if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
		return "bool"
	}
	switch t := fmt.Sprintf("%T", f.Value); {
	case strings.HasSuffix(t, "durationValue"):
		return "duration"
	case strings.HasSuffix(t, "intValue"), strings.HasSuffix(t, "int64Value"):
		return "int"
	case strings.HasSuffix(t, "multiFlag"), strings.HasSuffix(t, "listFlag"):
		return "stringSlice"
	case strings.HasSuffix(t, "stringValue"), strings.HasSuffix(t, "funcValue"):
		return "string"
	default:
		return "unknown:" + t
	}
}

func constructorFlags(key string) *flag.FlagSet {
	if e, ok := v15Subcommands[key]; ok && e.Flags != nil {
		return e.Flags()
	}
	if sub, ok := strings.CutPrefix(key, "runs "); ok && runsSubFlags[sub] != nil {
		return runsSubFlags[sub]()
	}
	return nil
}

// checkFlags compares spec flags with a constructor's flags; reverse also demands every constructor flag be declared.
func checkFlags(path string, row specRow, fs *flag.FlagSet, reverse bool) []string {
	var out []string
	declared := map[string]bool{}
	for _, f := range row.Flags {
		declared[f.Name] = true
		if f.Name == "post-status" && path == "root" {
			continue
		}
		got := fs.Lookup(f.Name)
		switch {
		case got == nil:
			out = append(out, fmt.Sprintf("%s: spec flag --%s is not defined by the Flags constructor", path, f.Name))
		case flagType(got) != f.Type:
			out = append(out, fmt.Sprintf("%s: flag --%s is %s in the code, %s in the spec", path, f.Name, flagType(got), f.Type))
		}
	}
	if reverse {
		fs.VisitAll(func(f *flag.Flag) {
			if !declared[f.Name] && f.Name != "help" && f.Name != "h" && f.Name != "sha" {
				out = append(out, fmt.Sprintf("%s: flag --%s is in the code but not in the spec", path, f.Name))
			}
		})
	}
	return out
}

func rankOf(class string) int { return classRank[class] }

// classProblems lists every way a row understates what the code behind it can do (surfaceClasses, SideEffect).
func classProblems(k string, r specRow) []string {
	var out []string
	c, ok := surfaceClasses[k]
	if !ok {
		return []string{k + ": no surfaceClasses entry: classify the command in cmd/surface_class.go"}
	}
	low := func(why, class string) {
		out = append(out, fmt.Sprintf("%s: side_effect %s is lower than %s (%s)", k, r.SideEffect, class, why))
	}
	if rankOf(r.SideEffect) < rankOf(c.Min) {
		low("the command", c.Min)
	}
	if e, ok := v15Subcommands[k]; ok && e.SideEffect != "" && rankOf(r.SideEffect) < rankOf(string(e.SideEffect)) {
		low("the table entry", string(e.SideEffect))
	}
	for sub, class := range c.Subs {
		if rankOf(r.SideEffect) < rankOf(class) {
			low("subcommand "+sub+" is reachable through the positional", class)
		}
	}
	declared := map[string]specFlag{}
	for _, f := range r.Flags {
		declared[f.Name] = f
	}
	fs := constructorFlags(k)
	for name, class := range c.Flags {
		if fs != nil && fs.Lookup(name) == nil {
			out = append(out, fmt.Sprintf("%s: surfaceClasses names flag --%s that the command does not define", k, name))
		}
		if f, ok := declared[name]; ok && !f.CLIOnly && rankOf(r.SideEffect) < rankOf(class) {
			out = append(out, fmt.Sprintf("%s: flag --%s reaches a %s action but the row is %s and the flag is not cli_only", k, name, class, r.SideEffect))
		}
	}
	return out
}

// specProblems is every disagreement between the spec and the code, as sorted lines naming the path.
func specProblems(s specFile) []string {
	rows := map[string]specRow{}
	for _, r := range s.Commands {
		rows[r.Path] = r
	}
	var problems []string
	keys := tableKeys()
	inTable := map[string]bool{}
	for _, k := range keys {
		inTable[k] = true
		r, ok := rows[k]
		if !ok {
			problems = append(problems, k+": table key has no spec row")
			continue
		}
		problems = append(problems, classProblems(k, r)...)
		// v1.4 rows (json none) keep the frozen v1.4 flags, which the V15 constructors need not define.
		if fs := constructorFlags(k); fs != nil && r.JSON == "envelope" {
			problems = append(problems, checkFlags(k, r, fs, true)...)
		}
	}
	for p := range rows {
		if !inTable[p] {
			problems = append(problems, p+": spec row has no table key")
		}
	}
	problems = append(problems, checkFlags("root", s.Root, bareFlags, true)...)
	if s.Root.SideEffect != "remote" || s.Root.Confirm == nil || len(s.Root.Confirm.Flags) != 0 {
		problems = append(problems, "root: bare run must be side_effect remote with confirm {flags: []}")
	}
	problems = append(problems, consentProblems(s)...)
	sort.Strings(problems)
	return problems
}

func TestManifestCommandsCurrent(t *testing.T) {
	s := loadSpec(t)
	problems := append(specProblems(s), manifestProblems(t, s)...)
	for _, p := range problems {
		t.Error(p)
	}
}

// manifestProblems compares the generated block in plugin.json with the spec rows (byte equality is in tools/manifestgen).
func manifestProblems(t *testing.T, s specFile) []string {
	raw, err := os.ReadFile("../plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Commands struct {
			SideEffect  string `json:"side_effect"`
			Subcommands []struct {
				Name       string `json:"name"`
				SideEffect string `json:"side_effect"`
				JSON       string `json:"json"`
				Surface    string `json:"surface"`
				Confirm    *struct {
					Flags []string `json:"flags"`
				} `json:"confirm"`
			} `json:"subcommands"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	var out []string
	if m.Commands.SideEffect != s.Root.SideEffect {
		out = append(out, "plugin.json root side_effect differs from the spec: run go generate ./tools/manifestgen/...")
	}
	got := map[string]int{}
	for i, c := range m.Commands.Subcommands {
		got[c.Name] = i
	}
	for _, r := range s.Commands {
		i, ok := got[r.Path]
		if !ok {
			out = append(out, r.Path+": missing from plugin.json commands")
			continue
		}
		c := m.Commands.Subcommands[i]
		if c.SideEffect != r.SideEffect || c.JSON != r.JSON || c.Surface != r.Surface || (c.Confirm == nil) != (r.Confirm == nil) {
			out = append(out, r.Path+": plugin.json differs from the spec")
		}
	}
	if len(m.Commands.Subcommands) != len(s.Commands) {
		out = append(out, "plugin.json lists a different number of commands than the spec")
	}
	return out
}

// consentProblems: every exposed remote or destructive row carries a confirm block, and every consent flag it names
// is a bool flag its command really defines. Rows with `flags: []` take the nonce round trip only and need no flag.
func consentProblems(s specFile) []string {
	var out []string
	for _, r := range s.Commands {
		risky := r.SideEffect == "remote" || r.SideEffect == "destructive"
		if risky && r.JSON == "envelope" && r.Surface != "cli-only" && r.Confirm == nil {
			out = append(out, fmt.Sprintf("%s: %s row is exposed without a confirm block", r.Path, r.SideEffect))
		}
		if r.Confirm == nil {
			continue
		}
		fs := constructorFlags(r.Path)
		for _, name := range r.Confirm.Flags {
			var f *flag.Flag
			if fs != nil {
				f = fs.Lookup(name)
			}
			if f == nil || flagType(f) != "bool" {
				out = append(out, fmt.Sprintf("%s: consent flag --%s is not a bool flag of the command", r.Path, name))
			}
		}
	}
	return out
}
