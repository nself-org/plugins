package dispatch

// Test-only exports for the external test package (dispatch_test).

// ResetSteps reopens step registration, keeping the named steps.
func ResetSteps(keep ...string) { resetSteps(keep...) }
