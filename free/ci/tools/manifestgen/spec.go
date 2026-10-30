package main

// Purpose: the typed form of free/ci/surface/commands.yaml, the one authored declaration of the ci plugin's machine
//          surface (P7-CI-62, CI D31), and its validation.
// Inputs:  the YAML bytes of the spec.
// Outputs: a validated Spec; every problem is returned at once, each naming the row path.
// Constraints: strict decode (an unknown key is an error); no I/O here; the rules mirror the registry's exposure
//              rules (SURF D2, D3): a remote or destructive row that is exposed (json envelope) needs a confirm block,
//              and an interactive row is cli-only.

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Arg is one positional argument in the registry Arg shape.
type Arg struct {
	Name     string `yaml:"name" json:"name"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Variadic bool   `yaml:"variadic,omitempty" json:"variadic,omitempty"`
	Secret   bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
}

// Flag is one flag in the registry Flag shape.
type Flag struct {
	Name       string `yaml:"name" json:"name"`
	Type       string `yaml:"type" json:"type"`
	Default    string `yaml:"default,omitempty" json:"default,omitempty"`
	Shorthand  string `yaml:"shorthand,omitempty" json:"shorthand,omitempty"`
	Usage      string `yaml:"usage,omitempty" json:"usage,omitempty"`
	SideEffect string `yaml:"side_effect,omitempty" json:"side_effect,omitempty"`
	Secret     bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
	CLIOnly    bool   `yaml:"cli_only,omitempty" json:"cli_only,omitempty"`
}

// Confirm names the consent flags that skip a command's prompt; an empty list means a nonce-only round trip.
type Confirm struct {
	Flags []string `yaml:"flags" json:"flags"`
}

// Row is one command of the surface.
type Row struct {
	Path       string   `yaml:"path,omitempty" json:"name,omitempty"`
	Summary    string   `yaml:"summary" json:"summary"`
	SideEffect string   `yaml:"side_effect" json:"side_effect"`
	Output     string   `yaml:"output" json:"output"`
	JSON       string   `yaml:"json" json:"json"`
	Surface    string   `yaml:"surface,omitempty" json:"surface,omitempty"`
	Confirm    *Confirm `yaml:"confirm,omitempty" json:"confirm,omitempty"`
	Args       []Arg    `yaml:"args,omitempty" json:"args,omitempty"`
	Flags      []Flag   `yaml:"flags,omitempty" json:"flags,omitempty"`
}

// Spec is the whole file: the binary, the bare root and one row per command key.
type Spec struct {
	Binary   string `yaml:"binary"`
	Command  string `yaml:"command"`
	Root     Row    `yaml:"root"`
	Commands []Row  `yaml:"commands"`
}

var (
	sideEffects = map[string]bool{"read": true, "write": true, "remote": true, "destructive": true}
	outputs     = map[string]bool{"document": true, "stream": true, "interactive": true}
	jsonKinds   = map[string]bool{"envelope": true, "none": true, "legacy": true}
	flagTypes   = map[string]bool{"bool": true, "string": true, "int": true, "duration": true, "stringSlice": true}
)

// Parse decodes and validates a spec.
func Parse(b []byte) (*Spec, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("surface spec: %w", err)
	}
	if problems := s.Problems(); len(problems) != 0 {
		return nil, fmt.Errorf("surface spec: %s", strings.Join(problems, "; "))
	}
	return &s, nil
}

// Problems lists every rule a spec breaks, sorted.
func (s *Spec) Problems() []string {
	var out []string
	if s.Binary == "" || s.Command == "" {
		out = append(out, "binary and command are required")
	}
	if s.Root.Path != "" {
		out = append(out, "root: must not have a path")
	}
	out = append(out, rowProblems("root", s.Root)...)
	seen := map[string]bool{}
	for _, r := range s.Commands {
		if r.Path == "" {
			out = append(out, "a command row has no path")
			continue
		}
		if seen[r.Path] {
			out = append(out, r.Path+": duplicate row")
		}
		seen[r.Path] = true
		out = append(out, rowProblems(r.Path, r)...)
	}
	sort.Strings(out)
	return out
}

func rowProblems(name string, r Row) []string {
	var out []string
	add := func(f string, a ...any) { out = append(out, name+": "+fmt.Sprintf(f, a...)) }
	if strings.TrimSpace(r.Summary) == "" {
		add("summary is required")
	}
	if !sideEffects[r.SideEffect] {
		add("side_effect %q is not read|write|remote|destructive", r.SideEffect)
	}
	if !outputs[r.Output] {
		add("output %q is not document|stream|interactive", r.Output)
	}
	if !jsonKinds[r.JSON] {
		add("json %q is not envelope|none|legacy", r.JSON)
	}
	if r.Surface != "" && r.Surface != "all" && r.Surface != "cli-only" {
		add("surface %q is not all|cli-only", r.Surface)
	}
	cliOnly := r.Surface == "cli-only"
	if r.Output == "interactive" && !cliOnly {
		add("an interactive command must be surface cli-only")
	}
	if (r.SideEffect == "remote" || r.SideEffect == "destructive") && r.JSON == "envelope" && r.Confirm == nil && !cliOnly {
		add("a %s command exposed on a machine surface needs a confirm block", r.SideEffect)
	}
	if r.Confirm != nil && r.SideEffect != "remote" && r.SideEffect != "destructive" {
		add("confirm is only for remote or destructive commands")
	}
	names := map[string]bool{}
	for _, f := range r.Flags {
		switch {
		case f.Name == "":
			add("a flag has no name")
		case names[f.Name]:
			add("flag %s is declared twice", f.Name)
		case !flagTypes[f.Type]:
			add("flag %s: type %q is not bool|string|int|duration|stringSlice", f.Name, f.Type)
		}
		names[f.Name] = true
	}
	if r.Confirm != nil {
		for _, c := range r.Confirm.Flags {
			if !names[c] {
				add("confirm flag %s is not a declared flag", c)
			}
		}
	}
	return out
}
