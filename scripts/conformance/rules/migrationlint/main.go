// Command migrationlint checks boot migration files for replay safety (P7 D-0346; used by the migration-idempotent and
// legacy-renames conformance rules).
//
//	migrationlint idempotent <file>...   prints "<file>:<line>: <message>" per violation, exit 1 on any
//	migrationlint renames <file>...      prints the sorted unique "old new" table renames of all files; exit 1 and a
//	                                     message on stderr when a rename cannot be read
//
// Exit 2 on a usage or I/O error. Standard library only.
package main

import (
	"fmt"
	"os"
	"sort"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) < 2 || (args[0] != "idempotent" && args[0] != "renames") {
		fmt.Fprintln(os.Stderr, "usage: migrationlint idempotent|renames <file>...")
		return 2
	}
	rc := 0
	seen := map[string]bool{}
	for _, f := range args[1:] {
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "migrationlint:", err)
			return 2
		}
		if args[0] == "idempotent" {
			for _, v := range lintFile(string(b)) {
				fmt.Printf("%s:%d: %s\n", f, v.Line, v.Msg)
				rc = 1
			}
			continue
		}
		ps, errs := renames(string(b))
		for _, p := range ps {
			seen[p] = true
		}
		for _, v := range errs {
			fmt.Fprintf(os.Stderr, "%s:%d: %s\n", f, v.Line, v.Msg)
			rc = 1
		}
	}
	if args[0] == "renames" {
		out := make([]string, 0, len(seen))
		for p := range seen {
			out = append(out, p)
		}
		sort.Strings(out)
		for _, p := range out {
			fmt.Println(p)
		}
	}
	return rc
}
