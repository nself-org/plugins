//go:build !windows

package hygiene

// Purpose: confined, no-follow removal of one attempt root.
// Inputs:  the base directory and the attempt name inside it.
// Outputs: the root and everything under it removed; nothing outside it touched.
// Constraints: every step is relative to an open directory handle (os.Root: openat and unlinkat
// with O_NOFOLLOW per component), and every opened directory is compared with the entry that was
// inspected (same file), so a symlink or mount swapped in during the walk cannot redirect it; a
// symlink is unlinked, never followed; a different device and, on Linux, any mount point listed in
// mountinfo under the root is refused; directories
// are made traversable first so a job that chmod-ed its tree to 0 cannot keep it alive.

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

const maxRemoveDepth = 512

// afterInspect, when set by a test, runs between inspecting the attempt entry and opening it, to
// swap the entry the way a hostile job would.
var afterInspect func()

// RemoveConfined removes base/name and everything beneath it. A path that escapes the root, or a
// base that is not a plain directory, is refused and logged through logf (which may be nil).
func RemoveConfined(base, name string, logf func(string, ...any)) error {
	say := func(format string, a ...any) {
		if logf != nil {
			logf(format, a...)
		}
	}
	if !ValidAttempt(name) {
		say("hygiene: refused %q: not a single safe path element", name)
		return leakError(ReasonUnconfined, "refused to remove a path that is not one safe element")
	}
	baseInfo, err := os.Lstat(base)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if baseInfo.Mode()&fs.ModeSymlink != 0 || !baseInfo.IsDir() {
		say("hygiene: refused %s: base is not a plain directory", base)
		return leakError(ReasonUnconfined, "job root base is not a plain directory")
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return err
	}
	defer root.Close()
	if opened, err := root.Stat("."); err != nil || !os.SameFile(baseInfo, opened) {
		say("hygiene: refused %s: base changed while opening", base)
		return leakError(ReasonUnconfined, "job root base changed while opening")
	}
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		// a lone file or symlink in the slot is unlinked itself, never followed
		return root.Remove(name)
	}
	if afterInspect != nil {
		afterInspect()
	}
	baseDev, _ := deviceOf(baseInfo)
	if d, ok := deviceOf(info); ok && d != baseDev {
		say("hygiene: refused %s/%s: another device (mount point)", base, name)
		return leakError(ReasonUnconfined, "the attempt root is a mount point")
	}
	real, err := filepath.EvalSymlinks(base)
	if err != nil {
		return leakError(ReasonUnconfined, "the job root base cannot be resolved")
	}
	mounts, ok := mountsUnder(filepath.Join(real, name))
	if !ok {
		say("hygiene: refused %s/%s: the mount table cannot be read", base, name)
		return leakError(ReasonUnconfined, "mount points under the attempt root cannot be checked")
	}
	if len(mounts) > 0 {
		say("hygiene: refused %s/%s: mount point inside the root: %s", base, name, mounts[0])
		return leakError(ReasonUnconfined, "a mount point is inside the attempt root")
	}
	sub, err := openDir(root, name, info)
	if err != nil {
		say("hygiene: refused %s/%s: %v", base, name, err)
		return leakError(ReasonUnconfined, "attempt root could not be opened without following links")
	}
	if opened, err := sub.Stat("."); err != nil || !os.SameFile(info, opened) {
		_ = sub.Close()
		say("hygiene: refused %s/%s: the entry changed while opening", base, name)
		return leakError(ReasonUnconfined, "the attempt root changed while opening")
	}
	failed, first := emptyDir(sub, baseDev, 0, say)
	_ = sub.Close()
	if failed == 0 {
		if err := root.Remove(name); err != nil {
			failed, first = failed+1, err
		}
	}
	if failed > 0 {
		return leakError(ReasonCleanupFailed, "%d entries under the attempt root could not be removed (first: %v)", failed, first)
	}
	return nil
}

// emptyDir removes every entry of r and returns how many could not be removed.
func emptyDir(r *os.Root, dev uint64, depth int, say func(string, ...any)) (int, error) {
	if depth > maxRemoveDepth {
		say("hygiene: refused: directory nesting beyond %d", maxRemoveDepth)
		return 1, leakError(ReasonUnconfined, "directory nesting too deep")
	}
	dir, err := r.Open(".")
	if err != nil {
		return 1, err
	}
	entries, err := dir.ReadDir(-1)
	_ = dir.Close()
	if err != nil {
		return 1, err
	}
	failed := 0
	var first error
	note := func(n int, err error) {
		if n > 0 {
			if failed == 0 {
				first = err
			}
			failed += n
		}
	}
	for _, e := range entries {
		name := e.Name()
		info, err := r.Lstat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			note(1, err)
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			if err := r.Remove(name); err != nil && !os.IsNotExist(err) {
				note(1, err)
			}
			continue
		}
		if d, ok := deviceOf(info); ok && d != dev {
			say("hygiene: refused %s: another device (mount point)", name)
			note(1, leakError(ReasonUnconfined, "mount point inside the attempt root"))
			continue
		}
		child, err := openDir(r, name, info)
		if err != nil {
			say("hygiene: refused %s: %v", name, err)
			note(1, err)
			continue
		}
		if opened, err := child.Stat("."); err != nil || !os.SameFile(info, opened) {
			_ = child.Close()
			say("hygiene: refused %s: the entry changed while opening", name)
			note(1, leakError(ReasonUnconfined, "an entry changed while opening"))
			continue
		}
		n, err := emptyDir(child, dev, depth+1, say)
		_ = child.Close()
		note(n, err)
		if n == 0 {
			if err := r.Remove(name); err != nil && !os.IsNotExist(err) {
				note(1, err)
			}
		}
	}
	return failed, first
}

// openDir opens the directory name of r. A directory the caller cannot enter is made traversable
// first, but only after it is confirmed to still be the entry that was inspected, so a name swapped
// for a link cannot have its target's mode changed.
func openDir(r *os.Root, name string, inspected fs.FileInfo) (*os.Root, error) {
	d, err := r.OpenRoot(name)
	if err == nil {
		return d, nil
	}
	if now, lerr := r.Lstat(name); lerr != nil || !os.SameFile(inspected, now) {
		return nil, err
	}
	if cerr := r.Chmod(name, 0o700); cerr != nil {
		return nil, err
	}
	return r.OpenRoot(name)
}

// deviceOf returns the device number of a stat result.
func deviceOf(info fs.FileInfo) (uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true //nolint:unconvert // int32 on darwin, uint64 on linux
}
