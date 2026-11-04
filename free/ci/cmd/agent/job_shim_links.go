package main

// Purpose: the link confinement of the job-shim tree stream (D-1230). A tree that crosses from the job
// user (stage-out) lands in the agent's workspace, so a symlink in it must never lead out of that
// workspace: checkLinkTarget refuses a target that is absolute or climbs above the tree when read from
// the link's own directory, and confineLinks resolves every link of the finished tree the way the kernel
// would (following the tree's own links, hop by hop) and refuses any that leaves it.
// Inputs:  an os.Root (the unpacked tree) and a link's name and target.
// Outputs: nil when every link stays inside the tree; an error naming the first link that does not.
// Constraints: fail closed. A chain of links that is inside at each step but escapes when followed
//          (deep/s -> ../.., deep/x -> s/..) is refused; so is a loop (more than maxLinkHops hops). A
//          name that does not exist yet is resolved lexically, so a dangling link is judged by where it
//          would point. Nothing here follows a link through the file system: links are read, never
//          entered.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// maxLinkHops is how many links one resolution may follow before it is refused as a loop (Linux: 40).
const maxLinkHops = 40

// checkLinkTarget refuses a target that is empty, absolute or climbs out of the tree from name's directory.
func checkLinkTarget(name, target string) error {
	if target == "" || path.IsAbs(target) || filepath.IsAbs(target) || strings.ContainsAny(target, "\\\x00") {
		return fmt.Errorf("tree link %q -> %q is absolute or not a plain relative path", name, target)
	}
	if j := path.Join(path.Dir(name), target); j == ".." || strings.HasPrefix(j, "../") {
		return fmt.Errorf("tree link %q -> %q leaves the directory", name, target)
	}
	return nil
}

// confineLinks resolves every symlink under root and refuses the first one whose resolution leaves it.
func confineLinks(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink == 0 {
			return err
		}
		return resolveInTree(root, p)
	})
}

// resolveInTree follows the link name inside root, component by component, and errors if any step leaves root.
func resolveInTree(root *os.Root, name string) error {
	target, err := root.Readlink(name)
	if err != nil {
		return err
	}
	if err := checkLinkTarget(name, target); err != nil {
		return err
	}
	var at []string // the resolved, link-free components from root
	if dir := path.Dir(name); dir != "." {
		at = strings.Split(dir, "/")
	}
	pending := strings.Split(target, "/")
	for hops := 0; len(pending) > 0; {
		c := pending[0]
		pending = pending[1:]
		switch c {
		case "", ".":
			continue
		case "..":
			if len(at) == 0 {
				return fmt.Errorf("tree link %q leaves the directory when followed", name)
			}
			at = at[:len(at)-1]
			continue
		}
		at = append(at, c)
		info, err := root.Lstat(strings.Join(at, "/"))
		if errors.Is(err, fs.ErrNotExist) {
			continue // not there (yet): judged lexically
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		if hops++; hops > maxLinkHops {
			return fmt.Errorf("tree link %q does not resolve (a loop of links)", name)
		}
		next, err := root.Readlink(strings.Join(at, "/"))
		if err != nil {
			return err
		}
		if next == "" || path.IsAbs(next) || filepath.IsAbs(next) || strings.ContainsAny(next, "\\\x00") {
			return fmt.Errorf("tree link %q leads through an absolute link", name)
		}
		at = at[:len(at)-1]
		pending = append(strings.Split(next, "/"), pending...)
	}
	return nil
}
