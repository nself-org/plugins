// Package schedpolicy defines, validates and types the scheduling policy keys
// (Epic SCHED D10, P7-SCHED-02). It registers every `sched.*` key with
// P7-TRUST-27's policy.RegisterKey from init(); the one loader, its stores and
// the merge stay in internal/policy and internal/trust.
//
// Value forms: booleans, string lists and strings are plain JSON; limits,
// costs and the queue depth are positive whole numbers; durations are Go
// duration strings ("10m") held as whole milliseconds. Zero and negative
// numbers are refused (E691) because the scheduler reads 0 as "no limit": a
// limit of 0 would silently lift the limit. No limit is the registered default
// (math.MaxInt64), which the typed view turns back into the scheduler's 0.
//
// Typed views: Placement (the sched.Policy the world builder hands to
// sched.Place), Queue (fairness and queue depth, read by the dispatcher) and
// ProviderLimits (per-provider limits; sched.Limits holds one number).
// Flags builds the revision-scope values of `--prefer`, `--runner` and
// `--max-queue`.
//
// Imports: internal/model, internal/policy, internal/sched, internal/trust.
package schedpolicy
