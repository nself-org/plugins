package diagnose

// Purpose: the infrastructure signatures P7-AUTO-16 adds to the CI-40 table:
// an unreachable provider or registry (timeouts and resets, as package
// managers and git report them) and the disk-full spellings the first row
// misses (ENOSPC, the Windows message).
// Inputs:  none (fixed rows, appended to the table at start-up). Outputs:
// SigNetwork and rows Classify tries after the CI-40 ones.
// Constraints: same rules as signatures.go (a row names one cause, matches an
// executor error or the last output lines, never a bare word); a refused
// connection is NOT a row: a service that is down on localhost is the
// project's own setup, not a broken CI.

// SigNetwork is the signature of an unreachable provider, registry or remote.
const SigNetwork = "infra.network"

func init() {
	signatures = append(signatures,
		signature{
			id:          SigNetwork,
			errText:     re(`(?i)\b(etimedout|esockettimedout|econnreset|eai_again|enetunreach|ehostunreach)\b|network is unreachable|tls handshake timeout|connection timed out|connection reset by peer|i/o timeout`),
			output:      re(`(?i)\b(etimedout|esockettimedout|econnreset|eai_again|enetunreach|ehostunreach)\b|network is unreachable|tls handshake timeout|connection timed out|connection reset by peer|socket hang up|err_pnpm_(meta_)?fetch\w*|npm err! network`),
			remediation: "A registry, provider or remote host could not be reached (timeout or reset). Check the runner's network and any proxy, then run it again.",
		},
		signature{
			id:          SigDisk,
			errText:     re(`(?i)\benospc\b|not enough space on the disk`),
			output:      re(`(?i)\benospc\b|not enough space on the disk|write error: no space`),
			remediation: remediationOf(SigDisk),
		},
	)
}

// remediationOf is the remediation of the first row with the id.
func remediationOf(id string) string {
	for _, s := range signatures {
		if s.id == id {
			return s.remediation
		}
	}
	return ""
}
