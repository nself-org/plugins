package main

import (
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run executes args and returns the process exit code. main and the tests both
// call it, so a test exercises the same wiring the binary has. stdout and stderr
// are read at call time, so a test can point them at files.
func run(args []string) int {
	rootCmd.SetArgs(args)
	ran = false
	return report(rootCmd.Execute(), wantsJSON(args), args, os.Stdout, os.Stderr)
}
