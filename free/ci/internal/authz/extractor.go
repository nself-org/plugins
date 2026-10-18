package authz

// Purpose: the stateful half of the path sanitiser. One Extractor judges the
// entries of ONE archive in order, so it can refuse what a single header never
// shows: a later entry written through an earlier symlink (symlink-then-file),
// two names that differ only by case, a repeated name, a file used as a
// directory, and cumulative entry, size and ratio bombs.
// Inputs:  Entry values, in archive order.
// Outputs: Admit returns the cleaned name or a *PathError (E683).
// Constraints: pure, not safe for concurrent use; one Extractor per archive.

import (
	"io/fs"
	"strings"
)

type nodeKind uint8

const (
	nodeDir nodeKind = iota + 1
	nodeFile
	nodeLink
)

type node struct {
	kind  nodeKind
	exact string // the cleaned name as written, to tell a repeat from a case clash
}

// Extractor holds the state of one archive.
type Extractor struct {
	lim    Limits
	nodes  map[string]node // fold key of the cleaned path -> what sits there
	count  int
	size   int64
	packed int64
}

// NewExtractor starts an archive with the given limits (zero fields default).
func NewExtractor(lim Limits) *Extractor {
	return &Extractor{lim: lim.norm(), nodes: map[string]node{}}
}

// Admit checks the next entry and records it on success. A refused entry is
// not recorded, but the caller must stop: an archive with one bad entry is bad.
func (x *Extractor) Admit(e Entry) (string, error) {
	clean, err := ArtifactPath(e, x.lim)
	if err != nil {
		return "", err
	}
	if x.count++; x.count > x.lim.MaxEntries {
		return "", reject(SizeEntries, e.Name)
	}
	if err := x.totals(e); err != nil {
		return "", err
	}
	if clean == "." {
		return clean, nil
	}
	if err := x.place(clean, kindOf(e.Mode), e.Name); err != nil {
		return "", err
	}
	return clean, nil
}

func kindOf(m fs.FileMode) nodeKind {
	switch m.Type() {
	case fs.ModeDir:
		return nodeDir
	case fs.ModeSymlink:
		return nodeLink
	}
	return nodeFile
}

// totals applies the cumulative size and ratio limits to regular files.
func (x *Extractor) totals(e Entry) error {
	if e.Mode.Type() != 0 {
		return nil
	}
	x.size += e.Size
	if e.Packed > 0 {
		x.packed += e.Packed
	}
	if x.size > x.lim.MaxTotalBytes {
		return reject(SizeTotal, e.Name)
	}
	return ratioCheck(x.size, x.packed, x.lim, e.Name)
}

// place records clean (and its implicit parents) or refuses a collision.
func (x *Extractor) place(clean string, kind nodeKind, name string) error {
	parts := strings.Split(clean, "/")
	for i := 1; i < len(parts); i++ {
		switch x.nodes[foldKey(strings.Join(parts[:i], "/"))].kind {
		case nodeLink:
			return reject(PathThroughSymlink, name)
		case nodeFile:
			return reject(PathParentNotDir, name)
		}
	}
	key := foldKey(clean)
	if old, seen := x.nodes[key]; seen {
		switch {
		case old.kind == nodeDir && kind == nodeDir && old.exact == clean:
			return nil // a repeated directory entry is harmless
		case old.exact == clean:
			return reject(CollisionDuplicate, name)
		}
		return reject(CollisionFold, name)
	}
	for i := 1; i < len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		if _, seen := x.nodes[foldKey(dir)]; !seen {
			x.nodes[foldKey(dir)] = node{nodeDir, dir}
		}
	}
	x.nodes[key] = node{kind, clean}
	return nil
}
