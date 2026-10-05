// Package surface holds the captured core `nself plugin <author command>` help.
//
// Purpose: `nself-plugin-dev <cmd> --help` prints what the core `nself plugin
// <cmd> --help` prints, so the plugin binary is a drop-in for the core
// commands (P7-CANON-18). The text is captured from a built core binary by
// scripts/capture-core-surface.sh, never typed by hand.
// Inputs:  a subcommand name: init, new, dev, debug, link, unlink or test.
// Outputs: the help text bytes with the usage path `nself plugin <cmd>` shown
// as `nself plugin-dev <cmd>` (the one normalisation the parity check allows).
// Constraints: the embedded copies must equal testdata/core-surface (unit test).
package surface

import (
	"embed"
	"regexp"
)

//go:embed help/*.txt
var helpFS embed.FS

// Names lists the subcommands that have captured help.
var Names = []string{"init", "new", "dev", "debug", "link", "unlink", "test"}

var pathRE = regexp.MustCompile(`nself plugin (init|new|dev|debug|link|unlink|test)\b`)

// Raw returns the captured core help bytes for name.
func Raw(name string) (string, bool) {
	b, err := helpFS.ReadFile("help/" + name + ".txt")
	if err != nil {
		return "", false
	}
	return string(b), true
}

// Rewrite maps the core usage path to the plugin command path.
func Rewrite(s string) string {
	return pathRE.ReplaceAllString(s, "nself plugin-dev $1")
}

// Help returns the help for name as the plugin binary prints it.
func Help(name string) (string, bool) {
	raw, ok := Raw(name)
	if !ok {
		return "", false
	}
	return Rewrite(raw), true
}
