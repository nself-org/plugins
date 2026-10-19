package secrets

// Purpose: the compat source for release/ANDROID_KEYSTORE_*: until release.age exists, an operator who
// exported the old variables still builds Android releases on their own machine (R-DX-12).
// Inputs:  a Ref and the process environment (injectable).
// Outputs: the value as a fresh byte slice, or false.
// Constraints: only the four names below, only class release, only when the Source decides the node is
// the operator's own personal node (the Source checks; a remote node never reaches this file). Using it
// is always audited and shows in evidence as source `legacy-env`.

import "os"

// LegacySource is the evidence label of a value that came from the legacy environment.
const LegacySource = "legacy-env"

// LegacyNames are the variables the compat source covers.
var LegacyNames = []string{"ANDROID_KEYSTORE_BASE64", "ANDROID_KEYSTORE_PASSWORD", "ANDROID_KEY_ALIAS", "ANDROID_KEY_PASSWORD"}

// LegacyEnv reads the legacy variables.
type LegacyEnv struct {
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
}

// Covers reports whether the compat source applies to the ref at all.
func (LegacyEnv) Covers(r Ref) bool {
	if r.Class != "release" {
		return false
	}
	for _, n := range LegacyNames {
		if r.Name == n {
			return true
		}
	}
	return false
}

// Lookup returns the legacy value of a covered ref when the variable is set and not empty.
func (l LegacyEnv) Lookup(r Ref) ([]byte, bool) {
	if !l.Covers(r) {
		return nil, false
	}
	get := l.Getenv
	if get == nil {
		get = os.Getenv
	}
	v := get(r.Name)
	if v == "" {
		return nil, false
	}
	return []byte(v), true
}
