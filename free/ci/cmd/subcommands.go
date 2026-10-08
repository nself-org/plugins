package main

// Purpose: the subcommand table of the nself-ci binary. Each cmd/<sub>.go
// registers its handler from init(), so a later Ticket (P7-CANON-10,
// P7-CI, P7-NODE) adds a file instead of editing main.go.
// Inputs:  registered keys (one or more space-separated tokens, for example
// "nodes provision") and argv.
// Outputs: the handler and remaining args for an argv; the default handler
// (the single-repo gate) when no key matches.
// Constraints: longest key wins; a key token never starts with "-"; keys are
// registered once (a duplicate is a programming error and panics at init).

import (
	"flag"
	"strings"
)

// handler runs one subcommand with the args that follow its key and returns
// the process exit code.
type handler func(args []string) int

// subcommands maps a space-joined key to its handler.
var subcommands = map[string]handler{}

// subcommandEntry carries v1.5-only metadata without changing the v1.4 table.
type subcommandEntry struct {
	Handler handler
	V15Only bool
	Hidden  bool
	Flags   func() *flag.FlagSet
}

var v15Subcommands = map[string]subcommandEntry{}

// maxKeyTokens is the longest registered key, in tokens.
var maxKeyTokens = 1

// register adds a subcommand. Called from init() in cmd/<sub>.go.
func register(key string, h handler) {
	if _, dup := subcommands[key]; dup {
		panic("nself-ci: duplicate subcommand " + key)
	}
	subcommands[key] = h
	if n := len(strings.Fields(key)); n > maxKeyTokens {
		maxKeyTokens = n
	}
}

// registerV15 makes a command reachable only from the v1.5 dispatcher.
func registerV15(key string, entry subcommandEntry) {
	if _, dup := v15Subcommands[key]; dup {
		panic("nself-ci: duplicate v1.5 subcommand " + key)
	}
	if !entry.V15Only || entry.Handler == nil {
		panic("nself-ci: invalid v1.5 subcommand " + key)
	}
	v15Subcommands[key] = entry
}

// resolve finds the handler for argv by longest match over the leading
// non-flag tokens. ok is false when no key matches (the caller runs the
// default gate). key is the matched key.
func resolve(args []string) (h handler, key string, rest []string, ok bool) {
	limit := maxKeyTokens
	if len(args) < limit {
		limit = len(args)
	}
	for n := limit; n >= 1; n-- {
		cand := args[:n]
		if hasFlagToken(cand) {
			continue
		}
		k := strings.Join(cand, " ")
		if h, found := subcommands[k]; found {
			return h, k, args[n:], true
		}
	}
	return nil, "", args, false
}

// hasFlagToken reports whether any token starts with "-".
func hasFlagToken(tokens []string) bool {
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") {
			return true
		}
	}
	return false
}
