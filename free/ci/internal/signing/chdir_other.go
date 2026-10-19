//go:build !unix

package signing

// Purpose: the non-unix side of chdir_unix.go.
// Inputs: a directory and an argv. Outputs: an E717 signer_failed refusal.
// Constraints: a call without a directory runs everywhere; one with a
// directory needs `sh`, so it is refused here.

func inDir(string, []string) ([]string, error) {
	return nil, Failf(ReasonSignerFailed, "running in a project directory needs a unix host")
}
