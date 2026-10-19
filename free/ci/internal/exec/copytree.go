package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// copySymlink recreates a symlink whose relative target stays inside the job
// source. The rule is syntactic on purpose: the target may climb with leading
// ".." elements, but never more levels than the link's own (real) directory is
// deep, and after that it may only descend. A ".." after a name is refused even
// when it cleans away lexically, because that name may itself be a symlink to a
// shallower directory and the ".." would then climb out of the source (for
// example a/x -> .. then a/l -> x/../..). Descending through other links is safe
// because each of them was vetted by the same rule. Absolute and empty targets
// are refused, so a job cannot reach files outside its workspace through a link.
func copySymlink(path, rel, target string) error {
	link, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if link == "" || filepath.IsAbs(link) || !linkStaysInside(link, rel) {
		return fmt.Errorf("symlink in job source: %s", rel)
	}
	return os.Symlink(link, target)
}

func linkStaysInside(link, rel string) bool {
	depth := 0
	if dir := filepath.Dir(rel); dir != "." {
		depth = len(strings.Split(dir, string(filepath.Separator)))
	}
	climbing := true
	for _, part := range strings.Split(filepath.ToSlash(link), "/") {
		switch {
		case part == ".." && climbing:
			depth--
			if depth < 0 {
				return false
			}
		case part == "..":
			return false
		case part != "" && part != ".":
			climbing = false
		}
	}
	return true
}

func copyTree(source, dest string) error {
	root, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			return copySymlink(path, rel, target)
		}
		if info.IsDir() {
			return os.Mkdir(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file in job source: %s", rel)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()&0700)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, err = out.ReadFrom(in)
		inputCloseErr := in.Close()
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return closeErr
	})
}
