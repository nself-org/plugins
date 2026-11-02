package auto

// Purpose: accept the -perf-json flag of the node scenario legs (P7-NODE-17, D-1530). The ticket verification runs
// `go test ./test/sim/... -args -perf-json <path>`; a package that does not define the flag exits 1 ("flag provided
// but not defined"), so this package registers it. It measures nothing and never writes the file.
// Inputs: -perf-json <path>. Outputs: none; the flag is a documented no-op here.
// Constraints: the name and usage match test/sim/perf_test.go, which owns the real measurements.

import "flag"

var _ = flag.String("perf-json", "", "accepted and ignored: the node scenario legs in test/sim write the perfbench/v1 measurements")
