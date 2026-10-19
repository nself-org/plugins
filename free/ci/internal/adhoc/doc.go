// Package adhoc runs `nself ci [flags] -- <cmd> [args…]`: one argv as a
// contained, evidenced single-job pipeline that never satisfies a gate
// (P7-CI-52, Epic D22).
//
// The package composes the normal run loop (internal/run) with a one-job plan;
// it has no run loop, no attempt creation and no runtime CLI call of its own
// beyond the service-network plumbing, which goes through the exec funnel.
package adhoc
