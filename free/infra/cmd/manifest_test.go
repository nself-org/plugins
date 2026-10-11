// Purpose: keep plugin.json's commands block and the cobra tree from drifting:
// every manifest subcommand exists, its flags match the cobra flags (name,
// type, default, usage) and every subcommand that takes --json declares it.
//
// Inputs: ../plugin.json (read only) and the command tree.
//
// Outputs: test results.
//
// Constraints: the pinned manifest schema accepts json none|legacy only (the
// k8s plugin has the same limit, debt D-0342 adds envelope), so a subcommand
// that prints a v1 envelope declares json legacy until that lands.
//
// json: legacy pending D-0342
package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type manifestFlag struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default string `json:"default"`
	Usage   string `json:"usage"`
}

type manifestSub struct {
	Name       string         `json:"name"`
	JSON       string         `json:"json"`
	SideEffect string         `json:"side_effect"`
	Surface    string         `json:"surface"`
	Flags      []manifestFlag `json:"flags"`
}

// manifestType maps a pflag type to the manifest's flag types.
func manifestType(t string) string {
	switch t {
	case "bool":
		return "bool"
	case "int64", "int":
		return "int"
	case "stringArray", "stringSlice":
		return "stringSlice" // repeatable
	}
	return "string" // string, duration
}

func TestManifestMatchesCommandTree(t *testing.T) {
	raw, err := os.ReadFile("../plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Commands struct {
			Subcommands []manifestSub `json:"subcommands"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	wantEffect := map[string]string{
		"validate": "read", "plan": "read", "apply": "remote", "destroy": "destructive",
		"server list": "read", "server provision": "remote", "server resize": "remote", "server destroy": "destructive",
	}
	declared := map[string]bool{}
	for _, sc := range m.Commands.Subcommands {
		declared[sc.Name] = true
		c, _, err := infraCmd.Find(strings.Fields(sc.Name))
		if err != nil || c == infraCmd {
			t.Errorf("manifest subcommand %q has no cobra command", sc.Name)
			continue
		}
		if sc.SideEffect != "read" && sc.Surface != "cli-only" {
			t.Errorf("%s: surface = %q, want cli-only (destructive or spending, no machine exposure)", sc.Name, sc.Surface)
		}
		// json: legacy pending D-0342
		if sc.JSON != "legacy" {
			t.Errorf("%s: json = %q, want legacy (it prints a v1 envelope)", sc.Name, sc.JSON)
		}
		if sc.SideEffect != wantEffect[sc.Name] {
			t.Errorf("%s: side_effect = %q, want %q", sc.Name, sc.SideEffect, wantEffect[sc.Name])
		}
		checkFlags(t, sc, c)
	}
	for name := range wantEffect {
		if !declared[name] {
			t.Errorf("subcommand %q is not in the manifest", name)
		}
	}
}

func checkFlags(t *testing.T, sc manifestSub, c *cobra.Command) {
	t.Helper()
	got := map[string]manifestFlag{}
	for _, f := range sc.Flags {
		got[f.Name] = f
	}
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" {
			return
		}
		mf, ok := got[f.Name]
		if !ok {
			t.Errorf("%s: cobra flag --%s is not in the manifest", sc.Name, f.Name)
			return
		}
		delete(got, f.Name)
		if mf.Type != manifestType(f.Value.Type()) {
			t.Errorf("%s --%s: type %q, cobra %q", sc.Name, f.Name, mf.Type, f.Value.Type())
		}
		def := f.DefValue
		if def == "" || def == "0" || def == "[]" {
			def = ""
		}
		if mf.Default != def {
			t.Errorf("%s --%s: default %q, cobra %q", sc.Name, f.Name, mf.Default, def)
		}
		if (f.Name == "json" || f.Name == "force") && mf.Usage != f.Usage {
			t.Errorf("%s --json: usage %q, cobra %q", sc.Name, mf.Usage, f.Usage)
		}
	})
	for name := range got {
		t.Errorf("%s: manifest flag --%s has no cobra flag", sc.Name, name)
	}
}
