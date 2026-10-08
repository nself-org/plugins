package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// runV15 selects the longest v1.5-only command or preserves a legacy key.
func runV15(args []string) int {
	if problems := model.CodeProblems(); len(problems) != 0 {
		fmt.Fprintln(os.Stderr, "invalid error-code registry:", strings.Join(problems, "; "))
		return 2
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "E603: V15 engine not built yet")
		return 2
	}
	for n := len(args); n >= 1; n-- {
		if hasFlagToken(args[:n]) {
			continue
		}
		if entry, ok := v15Subcommands[strings.Join(args[:n], " ")]; ok {
			return entry.Handler(args[n:])
		}
	}
	return run(args)
}
