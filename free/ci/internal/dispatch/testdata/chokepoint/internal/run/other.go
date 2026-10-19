package run

// Planted: LocalAdmit is only for CreateAttempt and the dispatch hook.
func Back() error { return LocalAdmit() }

// Planted: a function value of LocalAdmit.
func Value() func() error { return LocalAdmit }

// A METHOD named CreateAttempt is not run.CreateAttempt.
func (T) CreateAttempt(s *store.Store) error { return s.AdmitAndLease(ctx, "host", demand) }
