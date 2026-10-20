package update

// Purpose: replace an installed agent binary atomically and keep the previous one (hq NODE D11).
// Inputs:  the installed path and a verified staged file.
// Outputs: <bin> is the new binary and <bin>.prev the old one; Restore puts the old one back.
// Constraints: the new file is written next to the old one (same directory, fsynced, the old file's mode) and moved over it with one rename, so a
//          reader sees the old or the new file, never a mix, and a running agent keeps its inode; the old binary is hard-linked
//          to .prev first (copied when the filesystem cannot link) so the swap never leaves the node without a binary; no step
//          needs more rights than the agent user already has in its own bin directory.

import (
	"fmt"
	"io"
	"os"
)

// Suffixes of the files next to the installed binary.
const (
	PrevSuffix = ".prev"
	NewSuffix  = ".new"
	// PrevVersionSuffix names the file that records which version the kept .prev binary is.
	PrevVersionSuffix = PrevSuffix + ".version"
	// defaultMode is the mode of a first install; a replacement keeps the mode of the file it replaces (job users execute the agent).
	defaultMode os.FileMode = 0o755
)

// Swap installs the staged file at bin and keeps the file it replaces as bin.prev.
func Swap(bin, staged string) error {
	tmp := bin + NewSuffix
	_ = os.Remove(tmp)
	mode := defaultMode
	if st, err := os.Stat(bin); err == nil {
		mode = st.Mode().Perm() | 0o100
	}
	if err := copyFile(staged, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("E656: cannot stage the new agent next to %s: %w", bin, err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("E656: cannot set the mode of the new agent: %w", err)
	}
	if _, err := os.Lstat(bin); err == nil {
		prev := bin + PrevSuffix
		// The marker names the version of the file at .prev: a marker left from an earlier swap must not outlive it.
		_ = os.Remove(bin + PrevVersionSuffix)
		_ = os.Remove(prev)
		if err := os.Link(bin, prev); err != nil {
			if err := copyFile(bin, prev); err != nil {
				_ = os.Remove(tmp)
				return fmt.Errorf("E656: cannot keep the previous agent: %w", err)
			}
		}
	}
	if err := os.Rename(tmp, bin); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("E656: cannot place the new agent: %w", err)
	}
	return nil
}

// Restore puts bin.prev back at bin with one rename.
func Restore(bin string) error {
	if err := os.Rename(bin+PrevSuffix, bin); err != nil {
		return fmt.Errorf("E656: cannot restore the previous agent: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
