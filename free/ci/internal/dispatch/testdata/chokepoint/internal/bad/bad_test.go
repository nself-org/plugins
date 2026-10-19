package bad

// A test file may call the definers (CI-31 store tests, SCHED-18 race test,
// TRUST's CheckDispatcher): the scan reads non-test files only.
func helper(s *store.Store) error { return s.AdmitAndLease(ctx, "host", demand) }
