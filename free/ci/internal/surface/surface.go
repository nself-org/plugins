// Package surface holds the captured core `nself ci` help text.
//
// Purpose: `nself-ci --help` (and the subcommand helps) print exactly what the
// core `nself ci --help` prints, so the plugin binary is a drop-in for the core
// command (P7-CANON-09). The text is captured from a built core binary by
// scripts/capture-core-surface.sh, never typed by hand.
// Inputs:  a command key: "" for ci, or "build", "forgejo", "serve".
// Outputs: the help text bytes.
// Constraints: the embedded copies must equal testdata/core-surface (unit test).
// `eval` is listed in the captured ci help because core lists it; it is not
// ported here (P7-CI-44/45).
package surface

import (
	"embed"
)

//go:embed help/*.txt
var helpFS embed.FS

// helpFile maps a command key to its embedded file.
var helpFile = map[string]string{
	"":        "help/ci.txt",
	"build":   "help/ci-build.txt",
	"forgejo": "help/ci-forgejo.txt",
	"serve":   "help/ci-serve.txt",
}

// Help returns the captured core help for key, and false when none exists.
func Help(key string) (string, bool) {
	name, ok := helpFile[key]
	if !ok {
		return "", false
	}
	b, err := helpFS.ReadFile(name)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// Keys lists the command keys that have captured help.
func Keys() []string {
	return []string{"", "build", "forgejo", "serve"}
}
