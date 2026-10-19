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
	"math"
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
	links  bool // a symlink was admitted
	wide   bool // a non-ASCII name or link target was admitted
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
	if err := x.aliasRisk(e); err != nil {
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

// aliasRisk refuses an archive that holds both a symlink and a non-ASCII name
// (or link target). Unicode-normalizing file systems (APFS, HFS+) give "e"+U+0301
// and U+00E9 one directory, so a symlink placed under one spelling and a file
// written under the other make the lexical depth that checkLink relies on
// differ from the physical one. foldKey folds case only (no x/text in the
// module), so the safe rule is no mixing of the two, in either order.
func (x *Extractor) aliasRisk(e Entry) error {
	link := e.Mode.Type() == fs.ModeSymlink
	wide := !isASCII(e.Name) || (link && !isASCII(e.LinkTarget))
	if (x.links || link) && (x.wide || wide) {
		return reject(PathUnicodeLink, e.Name)
	}
	x.links = x.links || link
	x.wide = x.wide || wide
	return nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
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
	if e.Size > x.lim.MaxTotalBytes-x.size { // x.size <= MaxTotalBytes: no overflow
		return reject(SizeTotal, e.Name)
	}
	x.size += e.Size
	if e.Packed > 0 {
		x.packed = addSat(x.packed, e.Packed)
	}
	return ratioCheck(x.size, x.packed, x.lim, e.Name)
}

func addSat(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
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
