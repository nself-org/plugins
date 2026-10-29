package main

// Purpose: `nself ci <sub> --help` and `-h` print that subcommand's help and exit 0 before anything runs.
// Inputs:  a matched key and the args after it; the v1.4 and v1.5 subcommand tables.
// Outputs: help text on stdout and exit code 0.
// Constraints: the dispatchers (run, runV15) call answerHelp before the handler and before the nodes
// --coordinator client, so a help request never starts a run or sends a request; a "--" terminator
// ends the scan (wantsHelp); a group word with no handler of its own (config, nodes pools) lists its
// children instead of falling through to the gate.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/surface"
)

// coreHelp holds the keys whose help keeps core's exact behaviour (captured text, and a flag that
// does not parse before the help token is an error); each answers without running the command.
var coreHelp = map[string]func(args []string) int{}

// answerHelp prints the help of key for a help request and returns the exit code.
func answerHelp(key string, args []string) int {
	if h, ok := coreHelp[key]; ok {
		return h(args)
	}
	fmt.Print(subHelp(key))
	return 0
}

// subHelp returns the help text of a registered key: the captured core text when there is one, else
// the usage line, the flag defaults of its V15 entry and the child commands under the key.
func subHelp(key string) string {
	if txt, ok := surface.Help(key); ok {
		return txt
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: nself ci %s [flags]\n", key)
	if e, ok := v15Subcommands[key]; ok && e.Flags != nil {
		if fs := e.Flags(); fs != nil {
			prev := fs.Output()
			fs.SetOutput(&b)
			b.WriteString("\nFlags:\n")
			fs.PrintDefaults()
			fs.SetOutput(prev)
		}
	}
	if kids := childKeys(key); len(kids) > 0 {
		b.WriteString("\nCommands:\n")
		for _, k := range kids {
			b.WriteString("  " + k + "\n")
		}
	}
	return b.String()
}

// childKeys lists the registered keys one or more tokens below key, sorted and without duplicates.
func childKeys(key string) []string {
	seen := map[string]bool{}
	prefix := key + " "
	for k := range subcommands {
		if strings.HasPrefix(k, prefix) {
			seen[k] = true
		}
	}
	for k, e := range v15Subcommands {
		if strings.HasPrefix(k, prefix) && !e.Hidden {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// helpForGroup prints the child list of a leading word that names a group (for example "nodes pools")
// but no command of its own, and reports whether args named one. Only called when wantsHelp(args).
func helpForGroup(args []string) bool {
	var words []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			break
		}
		words = append(words, a)
	}
	for n := len(words); n >= 1; n-- {
		key := strings.Join(words[:n], " ")
		if kids := childKeys(key); len(kids) > 0 {
			fmt.Print("Usage: nself ci " + key + " <command> [flags]\n\nCommands:\n")
			for _, k := range kids {
				fmt.Println("  " + k)
			}
			return true
		}
	}
	return false
}
