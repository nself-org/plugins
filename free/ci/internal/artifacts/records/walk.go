package records

// Purpose: expand one declared path or glob inside the job workdir without
// ever following a symlink, and judge every entry with the shared sanitiser.
// Inputs:  an fs.FS rooted at the workdir (os.Root.FS: it refuses to leave
// the root), the cleaned declaration path.
// Outputs: the files (and refused entry types) the declaration selects.
// Constraints: components match with path.Match; a "**" component spans any
// number of directories; a matched directory selects every entry below it;
// symlinks are listed, never followed; Admit (authz.Extractor) refuses
// escapes, devices, FIFOs, collisions and bombs, and this file refuses
// hardlinks and every symlink that survives it. Packed is the file size: a
// stored file has no compression ratio, so the ratio rule never fires.

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

type found struct {
	rel  string
	info fs.FileInfo
}

func hasMeta(s string) bool { return strings.ContainsAny(s, "*?[") }

// expand lists the entries a declaration selects, sorted by path.
func expand(fsys fs.FS, pattern string, maxEntries int) ([]found, error) {
	seen := map[string]found{}
	var walk func(dir string, comps []string) error
	add := func(rel string) error {
		info, err := fs.Lstat(fsys, rel)
		if err != nil {
			return failf(ReasonMissing, "declared artifact %q: %v", rel, err)
		}
		if !info.IsDir() {
			seen[rel] = found{rel, info}
			return nil
		}
		return fs.WalkDir(fsys, rel, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return failf(ReasonUnreadable, "%s: %v", p, err)
			}
			if d.IsDir() {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return failf(ReasonUnreadable, "%s: %v", p, err)
			}
			if seen[p] = (found{p, fi}); len(seen) > maxEntries {
				return authzTooMany(p)
			}
			return nil
		})
	}
	walk = func(dir string, comps []string) error {
		if len(comps) == 0 {
			return add(dir)
		}
		comp, rest := comps[0], comps[1:]
		if comp == "**" {
			if err := walk(dir, rest); err != nil {
				return err
			}
			ents, err := fs.ReadDir(fsys, orDot(dir))
			if err != nil {
				return nil // a missing or unreadable directory selects nothing
			}
			for _, e := range ents {
				if e.IsDir() {
					if err := walk(join(dir, e.Name()), comps); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if !hasMeta(comp) {
			if _, err := fs.Lstat(fsys, join(dir, comp)); err != nil {
				return nil
			}
			return walk(join(dir, comp), rest)
		}
		ents, err := fs.ReadDir(fsys, orDot(dir))
		if err != nil {
			return nil
		}
		for _, e := range ents {
			if ok, merr := path.Match(comp, e.Name()); merr != nil {
				return failf(ReasonDeclaration, "artifact pattern %q: %v", pattern, merr)
			} else if ok {
				if err := walk(join(dir, e.Name()), rest); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk("", strings.Split(pattern, "/")); err != nil {
		return nil, err
	}
	out := make([]found, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, nil
}

func authzTooMany(p string) error {
	return &authz.PathError{Reason: authz.SizeEntries, Name: p}
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func orDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// admit judges one selected entry.
func admit(x *authz.Extractor, fsys fs.FS, f found) error {
	e := authz.Entry{Name: f.rel, Mode: f.info.Mode(), Size: f.info.Size(), Packed: f.info.Size(), Hardlink: f.info.Mode().IsRegular() && linkCount(f.info) > 1}
	if f.info.Mode().Type() == fs.ModeSymlink {
		target, err := fs.ReadLink(fsys, f.rel)
		if err != nil {
			return failf(ReasonUnreadable, "%s: %v", f.rel, err)
		}
		e.LinkTarget = target
	}
	if _, err := x.Admit(e); err != nil {
		return err
	}
	if !f.info.Mode().IsRegular() { // an in-tree symlink: admitted by the sanitiser, still not stored
		return &authz.PathError{Reason: authz.EntryIrregular, Name: f.rel}
	}
	return nil
}
