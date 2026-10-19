package diagnose

// Purpose: the infrastructure signatures P7-AUTO-16 adds to the CI-40 table:
// an unreachable provider or registry (timeouts and resets, as package
// managers and git report them) and the disk-full spellings the first row
// misses (ENOSPC, the Windows message).
// Inputs:  none (fixed rows, appended to the table at start-up). Outputs:
// SigNetwork and rows Classify tries after the CI-40 ones.
// ClassifyInstall judges the log of a failed dependency install (npm ci,
// pnpm/yarn/bun frozen installs), which Classify's last-three-lines window
// cannot: package managers print the cause well above their usage footer.
// Constraints: same rules as signatures.go (a row names one cause, matches an
// executor error or the last output lines, never a bare word); a refused
// connection is NOT a row: a service that is down on localhost is the
// project's own setup, not a broken CI.

// SigNetwork is the signature of an unreachable provider, registry or remote.
const SigNetwork = "infra.network"

const networkText = `(?i)\b(etimedout|esockettimedout|econnreset|eai_again|enetunreach|ehostunreach)\b|network is unreachable|tls handshake timeout|connection timed out|connection reset by peer|i/o timeout`

func init() {
	signatures = append(signatures,
		signature{
			id:          SigNetwork,
			errText:     re(networkText),
			output:      re(networkText + `|socket hang up|err_pnpm_meta_fetch_fail|err_pnpm_fetch_5\d\d|npm err! network`),
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
func remediationOf(id string) string { return signatureByID(id).remediation }

// InstallLines is how much of an install log ClassifyInstall reads.
const InstallLines = 60

// installSync marks a lockfile that disagrees with the manifest: always the
// project's own fault, whatever transient noise the manager printed before it.
var installSync = re(`(?i)err_pnpm_outdated_lockfile|err_pnpm_lockfile_config_mismatch|frozen-lockfile.{0,80}(not up to date|needs to be updated|would)|yn0028|lockfile would have been modified|lockfile had changes|lockfile needs to be updated|package\.json and package-lock\.json.{0,40}(in sync|not in sync)|npm ci. can only install packages when|eusage`)

// installTestFailure marks a lifecycle script (postinstall, prepare) that runs
// a test runner and reports its own assertion: infra-looking words around it
// are the program's. Narrower than testFailure: package managers say "failed"
// about the network too.
var installTestFailure = re(`(?m)^\s*(--- FAIL\b|FAIL\s|not ok\b)|✕|✗|assertion ?error|assertion failed|panic:`)

// installOnly are causes that only mean infrastructure inside an install: a
// registry that refuses the connection (the project has no server to start).
var installOnly = re(`(?i)\beconnrefused\b|connection refused|\benotfound\b`)

// ClassifyInstall classifies a failed dependency install from its log: infra
// (retryable, with the signature) for a full disk, DNS, an unreachable
// registry or a kill by memory; code for everything else, a lockfile out of
// sync always first.
func ClassifyInstall(log string) Diagnosis {
	window := lastLines(log, InstallLines)
	if installSync.MatchString(window) || installTestFailure.MatchString(window) {
		return named(SigCode, "code")
	}
	for _, s := range signatures {
		switch s.id {
		case SigDisk, SigDNS, SigNetwork, SigOOM:
			if s.output != nil && s.output.MatchString(window) || s.errText != nil && s.errText.MatchString(window) {
				return infra(s)
			}
		}
	}
	if installOnly.MatchString(window) {
		return infra(signatureByID(SigNetwork))
	}
	return named(SigCode, "code")
}

func signatureByID(id string) signature {
	for _, s := range signatures {
		if s.id == id {
			return s
		}
	}
	return signature{}
}
