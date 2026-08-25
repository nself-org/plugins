// Package projectroot reimplements the one function of the CLI's
// internal/config package that the waf plugin needs: finding the nSelf
// project root from the current working directory.
//
// Purpose: the plugin is its own Go module and cannot import
// github.com/nself-org/cli/internal/config (Go's internal/ visibility rule
// forbids it across module boundaries), so this file copies FindNSelfRoot
// byte-for-byte from internal/config/helpers.go (CLI-R11) rather than
// re-deriving the walk-up-and-check-markers logic independently. Keeping it
// a literal copy means a future change to the project-marker rules in core
// has an obvious, greppable twin to update here.
//
// Inputs: startDir, normally the caller's working directory.
//
// Outputs: the resolved project root directory, or an error if none is
// found within 10 levels or before reaching $HOME / the filesystem root.
//
// Constraints: no dependency beyond the standard library.
package projectroot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// projectMarkerFiles are the committed cascade files whose presence marks a
// directory as an nSelf project root. Kept in sync with
// internal/config/helpers.go's projectMarkerFiles.
var projectMarkerFiles = []string{".env", ".env.dev", ".env.staging", ".env.prod"}

// hasProjectMarker reports whether dir contains any committed cascade file.
func hasProjectMarker(dir string) bool {
	for _, name := range projectMarkerFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// FindNSelfRoot walks up from startDir looking for an nSelf project root.
// It checks, at each directory level:
//  1. startDir/.backend/.env* → returns startDir/.backend (monorepo case)
//  2. startDir/.env*          → returns startDir (already in backend dir)
//
// Walking stops at $HOME, at /, or after 10 levels — whichever comes first.
// Returns an error if no project root is found.
func FindNSelfRoot(startDir string) (string, error) {
	home, _ := os.UserHomeDir()

	dir := startDir
	for i := 0; i < 10; i++ {
		if dir == home || filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no nself project found: looked for %s in %s and each parent directory",
				strings.Join(projectMarkerFiles, ", "), startDir)
		}

		if hasProjectMarker(filepath.Join(dir, ".backend")) {
			return filepath.Join(dir, ".backend"), nil
		}

		if hasProjectMarker(dir) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no nself project found")
}
