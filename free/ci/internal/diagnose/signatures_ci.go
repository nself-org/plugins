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

// installSync marks a lockfile that disagrees with the manifest: always the
// project's own fault, whatever transient noise the manager printed before it.
var installSync = re(`(?i)err_pnpm_outdated_lockfile|err_pnpm_lockfile_config_mismatch|frozen-lockfile.{0,80}(not up to date|needs to be updated|would)|yn0028|lockfile would have been modified|lockfile had changes|lockfile needs to be updated|package\.json and package-lock\.json.{0,40}(in sync|not in sync)|npm ci. can only install packages when`)

// installTestFailure marks a lifecycle script (postinstall, prepare) that runs
// a test runner or an assertion and reports it. Narrower than testFailure:
// package managers say "failed" about the network too.
var installTestFailure = re(`(?im)^\s*(--- FAIL\b|FAIL\s|not ok\b)|✕|✗|assertion ?error|assertion failed|err_assertion|panic:`)

// installLifecycle marks a failing lifecycle script (preinstall, install,
// postinstall, prepare, a native build) in the shapes of npm, pnpm and both
// Yarns: the project's own command failed, and whatever it printed about
// connections is about the project's setup.
var installLifecycle = re(`(?i)\belifecycle\b|npm error command failed|command failed with exit code|(^|\n)command failed\.?\s*\n\s*exit code:|couldn't be built successfully|yn0009|(^|\n)> \S+@\S+ (pre|post)?(install|prepare)\b|script from .* failed|gyp err`)

// installRetry marks a transient the manager recovered from (or is about to
// retry): never the reason the install ended.
var installRetry = re(`(?i)\bwarn\b|will retry|retrying|retries left|\battempt \d`)

// installLocal marks a connection to this machine: refused or reset there is
// the project's setup. Read with URLs removed, so a package named `localhost`
// in a registry path does not count.
var installLocal = re(`(?i)\b127\.\d+\.\d+\.\d+\b|\blocalhost\b|(^|[\s\[(])::1([\]\s):]|$)`)

// installClientError marks an answer from a reachable registry that says the
// request itself is wrong (no such package or version, forbidden,
// unauthorized): configuration, never a transient. Read with URLs removed.
var installClientError = re(`(?i)\be?40[0-4]\b|\bnot found\b|\bforbidden\b|\bunauthorized\b|no matching version|\bnotarget\b|err_pnpm_no_matching_version`)

// installURL is stripped before the guards and causes are read: a package
// named `etimedout`, `enospc` or `localhost` in a registry URL is not a cause.
var installURL = re(`https?://\S+`)

// installDisk is a full disk, in the words every manager and the OS share.
var installDisk = re(`(?i)\benospc\b|no space left on device|not enough space on the disk`)

// installKilled is the shell naming a killed child (memory).
var installKilled = re(`(?m)^\S*sh: (line \d+: )? *\d+ +Killed\b`)

// Manager-structured causes: the line each manager prints for the failure that
// ended the install, never free text. The last such line decides.
var (
	installNPMCode  = re(`(?im)^npm (?:error|err!) code (\w+)`)
	installPNPMCode = re(`(?m)^\s*(ERR_PNPM_\w+)\s`)
	installYarnErr  = re(`(?im)^(?:error An unexpected error occurred:|➤ YN0001:).*\b(etimedout|esockettimedout|econnreset|econnrefused|enetunreach|ehostunreach|enotfound|eai_again)\b|^error An unexpected error occurred:.*socket hang up`)
	installBunErr   = re(`(?im)^error: (connectionrefused|connectionclosed|connectiontimedout|connectionreset|failedtoopensocket)\b`)
)

var installDNSCodes = map[string]bool{"ENOTFOUND": true, "EAI_AGAIN": true}
var installNetCodes = map[string]bool{"ETIMEDOUT": true, "ESOCKETTIMEDOUT": true, "ECONNRESET": true, "ECONNREFUSED": true, "ENETUNREACH": true, "EHOSTUNREACH": true}

// lastCode is the first capture group of the last match of r.
func lastCode(r *regexp.Regexp, text string) string {
	m := r.FindAllStringSubmatch(text, -1)
	if len(m) == 0 {
		return ""
	}
	return strings.ToUpper(m[len(m)-1][1])
}

// ClassifyInstall classifies a failed dependency install from its log: infra
// (retryable, with the signature) for a full disk, an unreachable registry
// (DNS or network) or a kill by memory; code for everything else, because an
// unknown install failure is the project's. Guards read the whole log first
// (a lockfile out of sync, a failing lifecycle script or test assertion, a
// connection to this machine, a client error from the registry) and return
// code; then the structured error lines of the managers, with URLs and retry
// warnings removed, name the cause.
func ClassifyInstall(log string) Diagnosis {
	log = installURL.ReplaceAllString(dropLines(log, installRetry), " ")
	for _, guard := range []*regexp.Regexp{installSync, installTestFailure, installLifecycle, installLocal, installClientError} {
		if guard.MatchString(log) {
			return named(SigCode, "code")
		}
	}
	switch {
	case installDisk.MatchString(log):
		return infra(signatureByID(SigDisk))
	case installKilled.MatchString(log):
		return infra(signatureByID(SigOOM))
	}
	code := lastCode(installNPMCode, log)
	if code == "" {
		if c := lastCode(installPNPMCode, log); c == "ERR_PNPM_META_FETCH_FAIL" || strings.HasPrefix(c, "ERR_PNPM_FETCH_5") {
			code = "ETIMEDOUT" // an unreachable or failing registry
		}
	}
	if code == "" {
		if m := installYarnErr.FindAllStringSubmatch(log, -1); len(m) > 0 {
			code = strings.ToUpper(m[len(m)-1][1])
			if code == "" {
				code = "ECONNRESET" // socket hang up
			}
		}
	}
	if code == "" && installBunErr.MatchString(log) {
		code = "ECONNREFUSED"
	}
	switch {
	case installDNSCodes[code]:
		return infra(signatureByID(SigDNS))
	case installNetCodes[code]:
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
