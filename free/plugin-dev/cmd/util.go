package main

// Purpose: small helpers shared by the author commands.
// Inputs/Outputs: see each function.
// Constraints: nselfBin resolves the core CLI the way core's own fallback does
// (the name "nself" on PATH); the plugin binary cannot use os.Executable for it
// because that is the plugin itself.

import (
	"os"
	"os/exec"
	"sort"
)

// sortedKeys returns the map keys in ascending order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// nselfBin returns the core CLI to run for `plugin install|remove`.
func nselfBin() string {
	if p, err := exec.LookPath("nself"); err == nil {
		return p
	}
	return "nself"
}

// selfBin returns this binary's path (core re-executes itself for `dev --debug`).
func selfBin() string {
	exe, err := os.Executable()
	if err != nil {
		return "nself-plugin-dev"
	}
	return exe
}
