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
	"unicode"
)

// SigNetwork is the signature of an unreachable provider, registry or remote.
const SigNetwork = "infra.network"

// SigPin is the signature of a package manager whose major version contradicts
// the packageManager pin; it is not retried.
const SigPin = "infra.package_manager_pin"

// The lines a Node gate script prints around a failed prepare (P7-AUTO-16).
// PrepareFailed ends a failed install's report and PinMismatch a refused pin,
// with the exit codes PrepareExit and PinExit. A report longer than 3500 bytes is
// the log's head, InstallGuardsMark and the first lines that carry a guard,
// InstallCausesMark and the last lines that carry a cause, InstallTailMark and
// the log's tail: a guard can never be evicted by a flood of cause lines.
const (
	PrepareFailed     = "::nself-ci-prepare-failed::"
	PinMismatch       = "::nself-ci-pin-mismatch::"
	InstallGuardsMark = "::nself-ci-install-guards::"
	InstallCausesMark = "::nself-ci-install-causes::"
	InstallTailMark   = "::nself-ci-install-tail::"
	PrepareExit       = 97
	PinExit           = 98
)

// InstallGuardWords and InstallCauseWords are the words (matched case-
// insensitively, as fixed strings, by the gate script's grep, warning lines
// excluded) of the lines ClassifyInstall's guards and causes read. No quote
// characters: the script quotes them for sh.
var (
	InstallGuardWords = []string{
		"elifecycle", "command failed", "built successfully", "yn0009", "assertion", "err_assertion", "--- fail", "not ok", "panic:",
		"lockfile", "yn0028", "in sync", "127.0.0.1", "localhost", "::1", "not found", "e401", "e402", "e403", "e404", "err_pnpm_fetch_40", "status 40", "http 40", "code 40", "error 40", "forbidden", "unauthorized",
		"no matching version", "notarget", "gyp err",
	}
	InstallCauseWords = []string{
		"npm error code", "npm err! code", "err_pnpm_", "unexpected error occurred", "yn0001", "enospc", "no space left", "not enough space",
		"killed", "error: connection", "error: failedtoopensocket", "dns error", "failed to lookup address",
	}
)

// providerNetwork matches a connection failure to a well-known package, source
// or image provider. Free text like "connection reset by peer" alone is NOT a
// signature for a job: a test that curls its own server and sees the reset is a
// code failure, and an infra class is retried (and can turn green). Installs
// have their own structured causes (ClassifyInstall).
const providerHostNames = `(?:proxy\.golang\.org|sum\.golang\.org|github\.com|raw\.githubusercontent\.com|registry\.npmjs\.org|registry\.yarnpkg\.com|pypi\.org|files\.pythonhosted\.org|crates\.io|static\.crates\.io|index\.docker\.io|registry-1\.docker\.io|ghcr\.io|gcr\.io|quay\.io)`

// providerHosts is a provider as a host: the authority of a URL, or a bare
// host name after whitespace or a quote; never a path segment, a longer host
// name or a suffix (`http://localhost:8080/registry.npmjs.org`).
const providerHosts = `(?:://(?:[^/\s@]*@)?|[\s'"(])` + providerHostNames + `(?:[:/\s'")]|$)`

const providerFailure = `(?:i/o timeout|connection timed out|connection reset by peer|tls handshake timeout|unexpected eof|network is unreachable|failed to connect|ssl connection timeout|recv failure)`

// providerNetwork: a provider host and a connection failure on one line, in either order.
var providerNetwork = `(?i)` + providerHosts + `[^\n]*` + providerFailure + `|` + providerFailure + `[^\n]*` + providerHosts

func init() {
	signatures = append(signatures,
		signature{
			id:          SigNetwork,
			errText:     re(providerNetwork),
			output:      re(providerNetwork),
			remediation: "A package, source or image provider could not be reached (timeout or reset). Check the runner's network and any proxy, then run it again.",
		},
		signature{
			id:          SigDisk,
			errText:     re(`(?im)(^|[\s:'"(])enospc([:\s'")]|$)|not enough space on the disk`),
			output:      re(`(?im)(^|[\s:'"(])enospc([:\s'")]|$)|not enough space on the disk|write error: no space`),
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
var installTestFailure = re(`(?im)^\s*(--- FAIL\b|FAIL\s|not ok\b)|assertion ?error|assertion failed|err_assertion|panic:`)

// installLifecycle marks a failing lifecycle script (preinstall, install,
// postinstall, prepare, a native build) in the shapes of npm, pnpm and both
// Yarns: the project's own command failed, and whatever it printed about
// connections is about the project's setup.
var installLifecycle = re(`(?i)\belifecycle\b|npm error command failed|\bcommand failed\b|couldn't be built successfully|yn0009|(^|\n)> \S+@\S+ (pre|post)?(install|prepare)\b|script from .* failed|gyp err`)

// installRetry marks a transient the manager recovered from (or is about to
// retry): never the reason the install ended.
var installRetry = re(`(?i)\bwarn\b|will retry|retrying|retries left|\battempt \d`)

// installLocal marks a connection to this machine: refused or reset there is
// the project's setup. Read with URLs removed, so a package named `localhost`
// in a registry path does not count.
var installLocal = re(`(?i)\b127\.\d+\.\d+\.\d+\b|\blocalhost\b|(^|[\s\[(])::1([\]\s):]|$)|\[::1\]`)

// installClientError marks an answer from a reachable registry that says the
// request itself is wrong (no such package or version, forbidden,
// unauthorized): configuration, never a transient. Read with URLs removed.
var installClientError = re(`(?i)\be40[0-4]\b|\b(?:http|status|code|error)[ :=]+40[0-4]\b|\b40[0-4]\s+(?:not found|forbidden|unauthorized|bad request|payment required)|err_pnpm_fetch_40\d|\bnot found\b|\bforbidden\b|\bunauthorized\b|no matching version|\bnotarget\b|err_pnpm_no_matching_version`)

// installURLHost captures the host of a URL: a registry on this machine is the
// project's own setup, but a package called `localhost` in a path is not.
var installURLHost = re(`(?i)https?://(?:[^/\s@]*@)?(\[[^\]]*\]|[^/\s:?#]+)`)

// installURL is stripped before the guards and causes are read: a package
// named `etimedout`, `enospc` or `localhost` in a registry URL is not a cause.
var installURL = re(`https?://\S+`)

// installDisk is a full disk, in the words every manager and the OS share.
// ENOSPC counts as a code (after whitespace, a colon or a quote), never as a path
// or package-name part (`/repo/enospc/package.json`).
var installDisk = re(`(?im)(^|[\s:'"(])enospc([:\s'")]|$)|no space left on device|not enough space on the disk`)

// installKilled is the shell naming a killed child (memory).
var installKilled = re(`(?m)^\S*sh: (line \d+: )? *\d+ +Killed\b`)

// Manager-structured causes: the line each manager prints for the failure that
// ended the install, never free text. The last such line decides.
var (
	installNPMCode  = re(`(?im)^npm (?:error|err!) code (\w+)`)
	installPNPMCode = re(`(?m)^\s*(?:Error:\s*)?(ERR_PNPM_\w+)\b`)
	installYarnErr  = re(`(?im)^(?:error An unexpected error occurred:|➤ YN0001:).*\b(etimedout|esockettimedout|econnreset|econnrefused|enetunreach|ehostunreach|enotfound|eai_again)\b|^error An unexpected error occurred:.*socket hang up`)
	installBunErr   = re(`(?im)^error: (connectionrefused|connectionclosed|connectiontimedout|connectionreset|failedtoopensocket)\b`)
)

// installDNSText is the chained cause pnpm 11+ prints for an unresolvable host.
var installDNSText = re(`(?i)dns error|failed to lookup address|name or service not known|temporary failure in name resolution`)

var installDNSCodes = map[string]bool{"ENOTFOUND": true, "EAI_AGAIN": true}

// installNetCodes are the codes of an unreachable or overloaded registry; E429 and
// E5xx are npm's codes for rate limits and gateway errors (retried, not the project's).
var installNetCodes = map[string]bool{"ETIMEDOUT": true, "ESOCKETTIMEDOUT": true, "ECONNRESET": true, "ECONNREFUSED": true, "ENETUNREACH": true, "EHOSTUNREACH": true,
	"E429": true, "E500": true, "E502": true, "E503": true, "E504": true}

// lastCode is the first capture group of the last match of r.
func lastCode(r *regexp.Regexp, text string) string {
	m := r.FindAllStringSubmatch(text, -1)
	if len(m) == 0 {
		return ""
	}
	return strings.ToUpper(m[len(m)-1][1])
}

// classifyPrepare is the verdict on a Node gate whose prepare step failed: the
// script's report (see InstallKeywords) is classified as an install log.
func classifyPrepare(report string) Diagnosis {
	d := ClassifyInstall(report)
	if d.Class == "infra" {
		d.Remediation = "E811: dependency install failed (" + d.Signature + "). " + d.Remediation
	}
	return d
}

// pinDiagnosis is the verdict on a refused package manager pin (E811).
func pinDiagnosis() Diagnosis {
	return Diagnosis{Class: "infra", Signature: SigPin, Remediation: "E811: package.json pins another major version of the package manager than the one installed. Install the pinned major (`corepack enable`), then run it again."}
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
	log = normalizeInstall(log)
	for _, m := range installURLHost.FindAllStringSubmatch(log, -1) {
		if installLocal.MatchString(" " + m[1] + " ") {
			return named(SigCode, "code")
		}
	}
	log = dropLines(installURL.ReplaceAllString(log, " "), installRetry) // URLs first: a package called `warn` is not a warning
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
		c := lastCode(installPNPMCode, log)
		switch {
		case installDNSText.MatchString(log) && (strings.HasPrefix(c, "ERR_PNPM_META_FETCH") || strings.HasPrefix(c, "ERR_PNPM_TARBALL_FETCH")):
			code = "ENOTFOUND" // pnpm 11+ names the chained cause: "dns error", "failed to lookup address"
		case strings.HasPrefix(c, "ERR_PNPM_META_FETCH") || strings.HasPrefix(c, "ERR_PNPM_TARBALL_FETCH") || c == "ERR_PNPM_FETCH_429" || strings.HasPrefix(c, "ERR_PNPM_FETCH_5"):
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

var installANSI = re(`\x1b\[[0-9;?]*[A-Za-z]`)

// normalizeInstall turns every Unicode space (pnpm pads codes with U+2009) into a
// plain space and removes terminal colour codes, so the patterns above see ASCII.
func normalizeInstall(log string) string {
	log = installANSI.ReplaceAllString(log, "")
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\r' && unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, log)
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
