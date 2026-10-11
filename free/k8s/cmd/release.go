// Purpose: validation of the --release value before it reaches helm.
//
// Inputs: the release name from --release.
//
// Outputs: nil, or a usage error (E401 under --json).
//
// Constraints: helm takes the release as a positional argument, so a value that
// starts with "-" would be read as a helm flag (--release=--dry-run). The rule
// is helm's own: lowercase alphanumerics and "-", starting and ending with an
// alphanumeric, at most 53 characters. uninstall (new, destructive) checks in
// both modes. install, upgrade and status apply the full rule only under
// --json: their human mode must stay byte-identical, so a name helm used to
// reject itself still reaches helm there. A name starting with "-" is refused
// in every mode of every command, because helm would parse it as a flag
// (`status --release --json` used to run `helm status --json`).
package main

import (
	"regexp"
	"strings"
)

const maxReleaseLen = 53

var releaseRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validReleaseName reports whether name is a legal helm release name.
func validReleaseName(name string) bool {
	return len(name) <= maxReleaseLen && releaseRe.MatchString(name)
}

// checkRelease returns a usage error for an illegal release name. always
// selects the modes: true checks human mode too, false only --json. A name
// starting with "-" fails in every mode.
func (r *invocation) checkRelease(name string, always bool) error {
	if validReleaseName(name) || (!r.json && !always && !strings.HasPrefix(name, "-")) {
		return nil
	}
	return r.usageFail("--release must be a helm release name: lowercase letters, digits and '-', starting and ending with a letter or digit, at most 53 characters")
}
