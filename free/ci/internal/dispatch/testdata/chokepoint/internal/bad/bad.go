package bad

// Planted violations: neither function is in internal/store, internal/lease,
// internal/dispatch or run.CreateAttempt.
func Sneak(s *store.Store) error { return s.AdmitAndLease(ctx, "host", demand) }

func Grantor(e *lease.Engine) { _, _ = e.Grant(ctx, "host", demand, 1) }

// Closure: the call belongs to the enclosing function, still a violation.
func Closure(s *store.Store) func() error { return func() error { return s.AdmitPlacement(ctx) } }
