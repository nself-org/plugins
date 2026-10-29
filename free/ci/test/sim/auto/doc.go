// Package auto holds the simulated legs of the AUTO conformance suite (P7-AUTO-25).
//
// Purpose: acceptance legs A3 to A7, A9 and A10 as deterministic Go tests that
// `free/ci/scripts/ci-conformance.sh --sim` runs one by one.
// Inputs: in-memory fakes of the provider, nodes and processes; no network.
// Outputs: pass or fail per test, named TestConformance<Leg>.
// Constraints: pure Go, no processes, no sleeps, no fixed ports; seeded generators print the seed.
package auto
