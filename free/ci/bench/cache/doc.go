// Package cache measures the cache engine (P7-CACHE-12). Every number it
// reports comes from a timed run of the real engine: cold and warm cycles of
// specs.Begin and specs.Session.Finalize on seeded fixture trees, in the real
// store and the real content-addressed blob store. Nothing is simulated.
//
// Purpose: record cold and warm restore and save times, hit rate, bytes and the
// integrity (re-hash) cost per fixture as a perfbench/v1 document that the
// cli's `perfbench check` reads, plus a detail section for the evidence file.
// Inputs:  Options (runs, seed, work dir) and the fixtures in fixtures.go.
// Outputs: Report (this package) and Result (package bench).
// Constraints: fixtures are generated from a seed, so two runs build identical
// trees; a restore that does not reproduce the saved tree byte for byte fails
// the run; remote carrier throughput is measured by the integration test
// TestRemoteCarrierThroughput in internal/cache/remote and merged by cachebench.
package cache
