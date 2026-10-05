// Command schemacheck validates plugin.json files against a JSON Schema (draft 2020-12).
//
// Usage: schemacheck <schema.json> <file.json>...
// Prints one line per violation ("<file>: <instance path>: <message>") and exits 1 when any file
// is invalid, 2 on a usage or I/O error. Used by scripts/conformance/rules/schema.sh against the
// pinned copy of cli's schemas/plugin-manifest.v2.schema.json.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func main() { os.Exit(run(os.Args[1:])) }

func load(path string) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return jsonschema.UnmarshalJSON(f)
}

func run(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: schemacheck <schema.json> <file.json>...")
		return 2
	}
	doc, err := load(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "schemacheck: %v\n", err)
		return 2
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", doc); err != nil {
		fmt.Fprintf(os.Stderr, "schemacheck: %v\n", err)
		return 2
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "schemacheck: compile: %v\n", err)
		return 2
	}
	rc := 0
	for _, file := range args[1:] {
		inst, err := load(file)
		if err != nil {
			fmt.Printf("%s: %v\n", file, err)
			rc = 1
			continue
		}
		if err := sch.Validate(inst); err != nil {
			rc = 1
			for _, l := range strings.Split(strings.TrimSpace(fmt.Sprintf("%v", err)), "\n") {
				fmt.Printf("%s: %s\n", file, strings.TrimSpace(l))
			}
		}
	}
	return rc
}
