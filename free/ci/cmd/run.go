package main

// Purpose: the "run" subcommand: discover .ci.yaml manifests under a search
// root and run all stages in canonical order (PLUGINS-CI-005).
// Inputs:  argv after "run": --env, --gateway, -v, --timeout, [search-root].
// Outputs: a stage table on stdout; exit code 0/1.
// Constraints: argv and output frozen until v1.6.0 (EPIC D11): deployed 1.4.x
// CLIs run this binary from plugins main.

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/legacy"
)

func init() {
	register("run", func(args []string) int { return runPipelineCmd(args) })
}

// runPipelineCmd implements "nself-ci run [flags] [search-root]".
// Discovers .ci.yaml manifests under search-root and runs all stages in canonical order.
// Adds a gateway routing check stage when --env or --gateway is provided.
func runPipelineCmd(rawArgs []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var (
		env        = fs.String("env", "", "Target environment: local")
		gatewayURL = fs.String("gateway", "", "Explicit gateway base URL (e.g. http://host:3761)")
		verbose    = fs.Bool("v", false, "Print each command before running")
		timeout    = fs.Int("timeout", 300, "Per-step timeout in seconds")
	)
	_ = fs.Parse(rawArgs)

	searchRoot := "."
	if fs.NArg() > 0 {
		searchRoot = fs.Arg(0)
	}
	if v := os.Getenv("NSELF_CI_SEARCH_ROOT"); v != "" && searchRoot == "." {
		searchRoot = v
	}

	// Resolve gateway base URL.
	gatewayBase, err := resolveGatewayBase(*env, *gatewayURL, os.Getenv("NSELF_CI_GATEWAY"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Printf("nself-ci pipeline — searching %s\n", searchRoot)
	if gatewayBase != "" {
		fmt.Printf("gateway routing check: %s\n", gatewayBase)
	}
	fmt.Println(strings.Repeat("─", 60))

	gates, err := legacy.DiscoverAndRunPipeline(searchRoot, gatewayBase, *timeout, *verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	if len(gates) == 0 {
		fmt.Println("No .ci.yaml manifests found under", searchRoot)
		return 0
	}

	if !printPipelineResults(gates) {
		return 1
	}
	return 0
}

// printPipelineResults prints a gate table and returns true if all passed.
func printPipelineResults(gates []legacy.GateResult) bool {
	allPassed := true
	for _, g := range gates {
		mark := "PASS"
		if !g.Passed {
			mark = "FAIL"
			allPassed = false
		}
		fmt.Printf("  %-45s  %s  (%s)\n", g.Name, mark, g.Elapsed.Round(time.Millisecond))
		if !g.Passed && g.Output != "" {
			for _, line := range strings.SplitAfter(g.Output, "\n") {
				fmt.Print("    ", line)
			}
			fmt.Println()
		}
	}
	fmt.Println(strings.Repeat("─", 60))
	overall := "PASSED"
	if !allPassed {
		overall = "FAILED"
	}
	fmt.Printf("  Pipeline: %s\n\n", overall)
	return allPassed
}
