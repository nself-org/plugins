package run

// Planted: LocalAdmit is only for CreateAttempt and the dispatch hook.
func Back() error { return LocalAdmit() }

// Planted: a function value of LocalAdmit.
func Value() func() error { return LocalAdmit }
