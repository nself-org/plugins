// Package fleet answers `ci nodes status` (P7-AUTO-19): it asks every registered node for its
// readiness and demand documents in parallel, under a hard cap per node, and merges the answers
// with the registry, the local run store and the pool of each node.
//
// A dead or slow node becomes an unreachable row with its reason and never delays the others.
// The package reaches hosts only through the Reader it is given (the probe package's fixed
// read-only commands over the SDK remote funnel); it opens no connection itself.
package fleet
