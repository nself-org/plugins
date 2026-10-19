package diagnose

// Purpose: the infrastructure signatures P7-AUTO-16 adds to the CI-40 table:
// an unreachable provider or registry (timeouts and resets, as package
// managers and git report them) and the disk-full spellings the first row
// misses (ENOSPC, the Windows message).
// Also ClassifyInstall, which judges the log of a failed dependency install
// (npm ci, pnpm/yarn/bun frozen installs): Classify's last-three-lines window
// cannot, because package managers print the cause well above their footer.
// Inputs:  none (fixed rows, appended to the table at start-up); an install log.
// Outputs: SigNetwork and rows Classify tries after the CI-40 ones; a Diagnosis.
// Constraints: same rules as signatures.go (a row names one cause, matches an
// executor error or the last output lines, never a bare word); a refused
// connection is NOT a table row: a service that is down on localhost is the
// project's own setup, so it counts as infrastructure only inside an install
// and only with a registry in view. Unknown stays code.

import (
	"regexp"
	"strings"
)

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

// InstallLines is how much of an install log ClassifyInstall matches causes in.
const InstallLines = 60

// installSync marks a lockfile that disagrees with the manifest: always the
// project's own fault, whatever transient noise the manager printed before it.
var installSync = re(`(?i)err_pnpm_outdated_lockfile|err_pnpm_lockfile_config_mismatch|frozen-lockfile.{0,80}(not up to date|needs to be updated|would)|yn0028|lockfile would have been modified|lockfile had changes|lockfile needs to be updated|package\.json and package-lock\.json.{0,40}(in sync|not in sync)|npm ci. can only install packages when`)

// installTestFailure marks a lifecycle script (postinstall, prepare) that runs
// a test runner or an assertion and reports it: infra-looking words around it
// are the program's. Narrower than testFailure: package managers say "failed"
// about the network too.
var installTestFailure = re(`(?im)^\s*(--- FAIL\b|FAIL\s|not ok\b)|✕|✗|assertion ?error|assertion failed|err_assertion|panic:`)

// installLifecycle marks a failing lifecycle script (preinstall, install,
// postinstall, prepare, a native build): the project's own command failed, and
// whatever it printed about connections is about the project's setup.
var installLifecycle = re(`(?i)\belifecycle\b|npm error command failed|command failed with exit code|couldn't be built successfully|yn0009|(^|\n)> \S+@\S+ (pre|post)?(install|prepare)\b|script from .* failed|gyp err`)

// installRetry marks a transient the manager recovered from (or is about to
// retry): never the reason the install ended.
var installRetry = re(`(?i)\bwarn\b|will retry|retrying|retries left|\battempt \d`)

// installLoopback marks a line about this machine: a refused or reset connection
// there is the project's setup, not an unreachable registry.
var installLoopback = re(`(?i)\b127\.\d+\.\d+\.\d+\b|\blocalhost\b|\[?::1\]?`)

// installRegistry is the context that lets a bare ENOTFOUND or ECONNREFUSED
// mean "the registry": the manager names a URL or a registry.
var installRegistry = re(`(?i)registry|https?://|npm error network|err_pnpm_(meta_)?fetch`)

// installOnly are causes that mean infrastructure only inside an install.
// Matched in the error-code position only (`code ENOTFOUND`, `connect ECONNREFUSED`),
// never inside a URL or a package name.
var installOnly = re(`(?i)\b(code|errno|connect|error|syscall|getaddrinfo)\s+(econnrefused|enotfound)\b|connection refused`)

// installNotFound marks a registry that answered "no such package or version":
// reachable, so never an unreachable-registry cause.
var installNotFound = re(`(?i)\b404\b|\be404\b|not found - get|no matching version|\bnotarget\b|err_pnpm_no_matching_version`)

// ClassifyInstall classifies a failed dependency install from its log: infra
// (retryable, with the signature) for a full disk, DNS, an unreachable
// registry or a kill by memory; code for everything else. The code guards read
// the whole log first (a lockfile out of sync, a failing lifecycle script or
// test assertion), then only the last InstallLines lines, minus retry warnings,
// loopback lines and registry "not found" answers, are matched for a cause.
func ClassifyInstall(log string) Diagnosis {
	if installSync.MatchString(log) || installTestFailure.MatchString(log) || installLifecycle.MatchString(log) {
		return named(SigCode, "code")
	}
	window := lastLines(dropLines(dropLines(dropLines(log, installRetry), installLoopback), installNotFound), InstallLines)
	for _, s := range signatures {
		switch s.id {
		case SigDisk, SigDNS, SigNetwork, SigOOM:
			if s.output != nil && s.output.MatchString(window) || s.errText != nil && s.errText.MatchString(window) {
				return infra(s)
			}
		}
	}
	if installOnly.MatchString(window) && installRegistry.MatchString(window) {
		return infra(signatureByID(SigNetwork))
	}
	return named(SigCode, "code")
}

// dropLines removes the lines that match r.
func dropLines(text string, r *regexp.Regexp) string {
	var keep []string
	for _, l := range strings.Split(text, "\n") {
		if !r.MatchString(l) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

func signatureByID(id string) signature {
	for _, s := range signatures {
		if s.id == id {
			return s
		}
	}
	return signature{}
}
