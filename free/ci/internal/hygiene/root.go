package hygiene

// Purpose: the per-attempt job root and the Host a node or the local run loop configures once.
// Inputs:  a base directory (<home>/work) and an attempt id.
// Outputs: a fresh 0700 directory owned by the caller, one per attempt.
// Constraints: the id is a safe single path element; an existing root is refused (attempt N+1 never
// inherits N's files); the base must not be a symlink.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var safeAttempt = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ValidAttempt reports whether id may name a job root and label resources.
func ValidAttempt(id string) bool { return safeAttempt.MatchString(id) && id != "." && id != ".." }

// Host is the node-level configuration every hygiene call shares.
type Host struct {
	// Base holds the attempt roots (<home>/work).
	Base string
	// Home is the CI home that holds process markers (<home>/processes); empty skips marker kills.
	Home string
	// Runtime lists and removes labelled containers, networks and volumes; nil uses the docker or
	// podman CLI through internal/exec.
	Runtime Runtime
	// Logf receives one line per refusal and per removal; nil discards them.
	Logf func(format string, args ...any)
	// MinAge protects a root younger than this from SweepOrphans (a sibling process may still be
	// creating it). The agent, which has authoritative leases, leaves it zero.
	MinAge time.Duration
	// ScanDirs are the directories LeakCheck searches for files owned by a job UID; nil uses the
	// temporary directories of the host.
	ScanDirs []string
	// DryRun reports what Cleanup would remove and removes nothing (tests).
	DryRun bool
}

func (h *Host) logf(format string, a ...any) {
	if h.Logf != nil {
		h.Logf(format, a...)
	}
}

func (h *Host) runtime() Runtime {
	if h.Runtime != nil {
		return h.Runtime
	}
	return defaultRuntime
}

// JobRoot creates the 0700 root of one attempt under base and returns its path.
func JobRoot(base, attempt string) (string, error) {
	if !ValidAttempt(attempt) {
		return "", leakError(ReasonBadAttempt, "invalid attempt id")
	}
	if base == "" || !filepath.IsAbs(base) {
		return "", leakError(ReasonUnconfined, "job root base must be an absolute path")
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(base)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return "", leakError(ReasonUnconfined, "job root base is not a plain directory")
	}
	root := filepath.Join(base, attempt)
	if err := os.Mkdir(root, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", leakError(ReasonLeak, "job root for attempt %s already exists", attempt)
		}
		return "", err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", fmt.Errorf("tighten job root: %w", err)
	}
	return root, nil
}

// JobRoot creates the root of one attempt under the host's base.
func (h *Host) JobRoot(attempt string) (string, error) { return JobRoot(h.Base, attempt) }
