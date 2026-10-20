package sbom

// Purpose: keep the release.sbom entry of a job's run out of the command the
// executor starts: the entry names a check, not a program.
// Inputs:  job.Run.
// Outputs: the command without a trailing release.sbom entry.
// Constraints: pure; the input is never modified; only the last entry counts
// (RequireForRelease refuses the check anywhere else in a release job).

// StripCheck returns run without its trailing release.sbom entry.
func StripCheck(run []string) []string {
	if n := len(run); n > 0 && stepRuns(run[n-1]) {
		return append([]string{}, run[:n-1]...)
	}
	return run
}
