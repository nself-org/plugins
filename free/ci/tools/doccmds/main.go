// Command doccmds runs the bash blocks of doc pages in clean fixture projects and fails when a block
// exits in a way its marker does not allow. A block under <!-- doc-command: expect-fail --> must exit
// non-zero; every other block must exit 0. It is run by .github/scripts/check-doc-commands.sh.
//
// Usage: doccmds <page.md>...
// Exit codes: 0 all blocks behave, 1 a block or page failed, 2 usage.
package main

import (
	"fmt"
	"io"
	"os"
)

// usageText is printed when no page is given.
const usageText = "usage: check-doc-commands.sh <page.md>..."

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run checks every page and writes one line per failure to stdout and page errors to stderr. It returns
// the process exit code.
func run(pages []string, stdout, stderr io.Writer) int {
	if len(pages) == 0 {
		fmt.Fprintln(stderr, usageText)
		return 2
	}
	opts := Options{Runner: shellRunner{timeout: blockTimeout}, Fixture: newFixture}
	status := 0
	for _, page := range pages {
		failures, err := CheckPage(page, opts)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			status = 1
			continue
		}
		for _, f := range failures {
			fmt.Fprintln(stdout, f.Error())
			status = 1
		}
	}
	return status
}
