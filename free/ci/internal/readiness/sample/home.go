package sample

// Purpose: where the CI home is, the directory every readiness path hangs from.
// Inputs: NSELF_CI_HOME, else ~/.nself/ci (the same default the exec funnel uses for jobs).
// Outputs: CIHome, and nearestExisting for a path whose directories may not exist yet.
// Constraints: no directory is created here.

import (
	"os"
	"path/filepath"
)

// CIHome returns the CI home directory ("" when the home directory cannot be found).
func CIHome() string {
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".nself", "ci")
}

// nearestExisting returns path, or its nearest existing ancestor: statfs needs a directory
// that exists, and a fresh host has no CI home yet but its volume is still the right one.
func nearestExisting(path string) string {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil || filepath.Dir(p) == p {
			return p
		}
	}
}
