// nself-plugin-dev: the plugin author tools, broken out of core.
//
// Purpose: `nself plugin-dev init|new|dev|debug|link|unlink|test` reproduce the
// core `nself plugin init|new|dev|debug|link|unlink|test` commands byte for
// byte on fixtures (P7-CANON-18): same flags, messages, exit codes and file
// effects. `new` is the deprecated alias of `init` with core's cobra notice.
//
// Usage:
//
//	nself-plugin-dev init <name> [--template go|rust|node|static] [--tier free|pro] ...
//	nself-plugin-dev new <name>           — deprecated alias of init
//	nself-plugin-dev dev <name> [--no-link] [--debug] [--entrypoint ./cmd]
//	nself-plugin-dev debug <name> [--port N] [--port-only]
//	nself-plugin-dev link <local-path> [--host] [--list]
//	nself-plugin-dev unlink <name>
//	nself-plugin-dev test <name> [--phase unit|smoke|both] [--host] [--no-cleanup]
//
// SPORT: PLUGINS-PLUGIN-DEV-000
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/nself-plugin-dev/internal/surface"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches argv and returns the exit code. The first argument selects
// the subcommand (or its alias); a bare invocation or -h/--help prints the
// command list.
func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(rootHelp())
		return 0
	}
	c, ok := lookup(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown command %q for %q\n", args[0], "nself plugin-dev")
		return 1
	}
	return c.execute(args[1:])
}

// execute runs one subcommand the way cobra does: the notice a deprecated
// command prints on stderr, then flag parsing, then help, then the argument
// check, then the command body. A returned error prints "Error: <msg>" and exits 1.
func (c *command) execute(argv []string) int {
	if c.deprecated != "" {
		fmt.Fprintf(os.Stderr, "Command %q is deprecated, %s\n", c.name, c.deprecated)
	}
	p, err := parse(c.flags, argv)
	if err != nil {
		return fail(err)
	}
	if p.help {
		txt, _ := surface.Help(c.name)
		fmt.Print(txt)
		return 0
	}
	if err := exactOneArg(p.args); err != nil {
		return fail(err)
	}
	if err := c.run(p); err != nil {
		return fail(err)
	}
	return 0
}

// fail prints the core error line and returns the exit status: the error's own
// ExitCode() when it has one (a failed child process: go, dlv, the watcher),
// else 1, as core's v1.4 reporter does.
func fail(err error) int {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

// rootHelp lists the subcommands.
func rootHelp() string {
	var b strings.Builder
	b.WriteString("Plugin author tools: scaffold, link, run, debug and test a plugin under development.\n\nUsage:\n  nself plugin-dev [command]\n\nAvailable Commands:\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-8s %s\n", c.name, c.short)
	}
	b.WriteString("\nUse \"nself plugin-dev [command] --help\" for more information about a command.\n")
	return b.String()
}
