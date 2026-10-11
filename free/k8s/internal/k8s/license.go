// Purpose: the licence key reaches plugin pods through a Kubernetes Secret
// that the user creates, never through helm values (debt D-0311): helm stores
// every value in the release revision Secret, and a value on argv is visible in
// the process list. install and upgrade take the Secret NAME (--license-secret)
// and write it to the overlay as license.secretName; the chart then gives every
// plugin workload the env NSELF_PLUGIN_LICENSE_KEY from that Secret.
//
// Inputs: the Secret name, and the licence key read from the environment only
// to know whether to warn.
//
// Outputs: ValidateLicenseSecret, LicenseKeyUnsent, LicenseSecretHint.
//
// Constraints: the key value is never stored, passed or printed here.
package k8s

import (
	"fmt"
	"regexp"
)

// LicenseEnv is the env var that holds the licence key on the CLI side and in
// the Secret (the chart's license.secretKey default).
const LicenseEnv = "NSELF_PLUGIN_LICENSE_KEY"

var dns1123Subdomain = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// ValidateLicenseSecret accepts an empty name (no licence Secret) or a DNS-1123
// subdomain, which is what a Kubernetes Secret name must be.
func ValidateLicenseSecret(name string) error {
	if name == "" {
		return nil
	}
	if len(name) > 253 || !dns1123Subdomain.MatchString(name) {
		return fmt.Errorf("k8s: --license-secret %q is not a valid Secret name (lower-case letters, digits, '-' and '.', starting and ending with a letter or digit, at most 253 characters)", name)
	}
	return nil
}

// LicenseKeyUnsent reports that a licence key is set in the environment but no
// --license-secret was given, so the key does not reach the cluster.
func LicenseKeyUnsent(opts InstallOptions) bool {
	return opts.LicenseKey != "" && opts.LicenseSecret == ""
}

// LicenseSecretHint is the warning for LicenseKeyUnsent. It names no key value.
const LicenseSecretHint = LicenseEnv + " is set but is not sent to the cluster. Create a Secret without putting the key on a command line, " +
	"for example: kubectl create secret generic nself-license --from-env-file=<file holding " + LicenseEnv + "=...>, " +
	"then pass --license-secret nself-license on every install and upgrade"
