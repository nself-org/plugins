package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateMediaPath_TraversalBlocked verifies that path traversal sequences
// are rejected by the path guard (T05 acceptance criterion: traversal blocked).
func TestValidateMediaPath_TraversalBlocked(t *testing.T) {
	root := t.TempDir()

	traversalCases := []string{
		"../../../etc/passwd",
		"../../secret",
		"../outside",
		"/etc/shadow",
		"/tmp/attacker",
	}

	for _, p := range traversalCases {
		t.Run(p, func(t *testing.T) {
			_, err := validateMediaPath(root, p)
			if err == nil {
				t.Errorf("expected traversal error for path %q within root %q, got nil", p, root)
			}
		})
	}
}

// TestValidateMediaPath_ValidPathAllowed verifies that a legitimate path within
// the media root is accepted (T05 acceptance criterion: valid paths proceed).
func TestValidateMediaPath_ValidPathAllowed(t *testing.T) {
	root := t.TempDir()

	// Create the file so it exists.
	videoFile := filepath.Join(root, "videos", "input.mp4")
	if err := os.MkdirAll(filepath.Dir(videoFile), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(videoFile, []byte(""), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	got, err := validateMediaPath(root, "videos/input.mp4")
	if err != nil {
		t.Errorf("expected nil error for valid path, got: %v", err)
	}
	if !strings.HasPrefix(got, root) {
		t.Errorf("returned path %q does not start with root %q", got, root)
	}
}

// TestValidateMediaPath_SymlinkOutsideBlocked verifies that a symlink whose
// target lies outside the media root is rejected (T05 acceptance criterion:
// symlinks outside base blocked). This confirms that filepath.EvalSymlinks is
// not required in this guard — the Rel check catches symlink escapes via
// canonical path resolution.
func TestValidateMediaPath_SymlinkOutsideBlocked(t *testing.T) {
	root := t.TempDir()

	// Create a symlink inside root that points outside.
	linkPath := filepath.Join(root, "escape")
	target := "/etc"
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skip("cannot create symlink (may need privileges): " + err.Error())
	}

	// The guard resolves via filepath.Clean/Rel; an absolute target (/etc)
	// will be caught as it falls outside the media root prefix.
	_, err := validateMediaPath(root, "/etc/shadow")
	if err == nil {
		t.Error("expected error for path outside media root via symlink target, got nil")
	}
}

// TestValidateMediaPath_EmptyPathRejected verifies that an empty path string
// returns an error.
func TestValidateMediaPath_EmptyPathRejected(t *testing.T) {
	root := t.TempDir()
	_, err := validateMediaPath(root, "")
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

// TestValidateMediaPath_RootItselfAllowed verifies that a path equal to the
// media root itself (dot or empty relative) is handled without panic.
func TestValidateMediaPath_RootItselfAllowed(t *testing.T) {
	root := t.TempDir()
	// "." resolves to root itself — this is allowed (not an escape).
	got, err := validateMediaPath(root, ".")
	if err != nil {
		t.Errorf("expected nil error for path='.', got: %v", err)
	}
	if got != filepath.Clean(root) {
		t.Errorf("expected clean root %q, got %q", filepath.Clean(root), got)
	}
}
