package authz

// Purpose: the archive and artifact path sanitiser (R-SEC-29; Epic CACHE D5/D6).
// Every tar header of a cache restore and every collected artifact path goes
// through ArtifactPath before anything is written.
// Inputs:  one Entry (name, link target, mode, hardlink flag, sizes) and Limits.
// Outputs: the cleaned relative name, or a *PathError (E683) naming the reason.
// Constraints: pure; no I/O. Allowed: regular files, directories and relative
// symlinks that stay inside the tree. Refused: absolute and drive names, NUL
// and control bytes, backslashes, any "..", hardlinks, devices, FIFOs, sockets,
// set-id bits, oversize and ratio bombs. A symlink target may only climb
// ("../") at its start and then descends, so lexical containment equals
// physical containment even through other in-tree symlinks. The stateful
// checks (collisions, writing through a symlink, totals) live in Extractor.

import (
	"fmt"
	"io/fs"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Entry is one archive header or collected file.
type Entry struct {
	Name       string
	LinkTarget string      // symlink target as stored
	Mode       fs.FileMode // type bits and permissions
	Hardlink   bool        // a hard link entry; the CALLER sets it from the tar typeflag (TypeLink), since FileInfo().Mode() of a hard link reads as a regular file
	Size       int64       // uncompressed bytes
	Packed     int64       // compressed bytes this entry consumed; <= 0 means unknown
}

// PathReason is a stable key for a refused entry.
type PathReason string

const (
	PathEmpty          PathReason = "path.empty"
	PathControl        PathReason = "path.control"
	PathUTF8           PathReason = "path.utf8"
	PathAbsolute       PathReason = "path.absolute"
	PathDrive          PathReason = "path.drive"
	PathBackslash      PathReason = "path.backslash"
	PathTraversal      PathReason = "path.traversal"
	PathDots           PathReason = "path.dots"
	PathDotGit         PathReason = "path.dot_git"
	PathUnicodeLink    PathReason = "path.unicode_with_symlink"
	PathEmptyComponent PathReason = "path.empty_component"
	PathTrailingSlash  PathReason = "path.trailing_slash"
	PathTooLong        PathReason = "path.too_long"
	PathTooDeep        PathReason = "path.too_deep"
	PathThroughSymlink PathReason = "path.through_symlink"
	PathParentNotDir   PathReason = "path.parent_not_dir"
	EntryHardlink      PathReason = "entry.hardlink"
	EntryDevice        PathReason = "entry.device"
	EntryFIFO          PathReason = "entry.fifo"
	EntrySocket        PathReason = "entry.socket"
	EntryIrregular     PathReason = "entry.irregular"
	EntrySetID         PathReason = "entry.setid"
	LinkEmpty          PathReason = "link.empty"
	LinkAbsolute       PathReason = "link.absolute"
	LinkEscape         PathReason = "link.escape"
	LinkInvalid        PathReason = "link.invalid"
	SizeNegative       PathReason = "size.negative"
	SizeFile           PathReason = "size.file"
	SizeTotal          PathReason = "size.total"
	SizeEntries        PathReason = "size.entries"
	SizeRatio          PathReason = "size.ratio"
	CollisionFold      PathReason = "collision.fold"
	CollisionDuplicate PathReason = "collision.duplicate"
)

// PathError is the error form of a refused entry; it carries code E683.
type PathError struct {
	Reason PathReason
	Name   string
}

func (e *PathError) Error() string {
	return fmt.Sprintf("%s: artifact path rejected: %s (%q)", CodeArtifactPath, e.Reason, e.Name)
}

func reject(r PathReason, name string) error { return &PathError{Reason: r, Name: name} }

// ArtifactPath checks one entry in isolation and returns its cleaned
// slash-separated name relative to the extraction root ("." for the root
// directory entry). Use an Extractor for a whole archive.
func ArtifactPath(e Entry, lim Limits) (string, error) {
	lim = lim.norm()
	if err := checkType(e); err != nil {
		return "", err
	}
	parts, err := splitName(e.Name, e.Mode.IsDir(), lim)
	if err != nil {
		return "", err
	}
	if e.Mode.Type() == fs.ModeSymlink {
		if len(parts) == 0 {
			return "", reject(PathEmpty, e.Name)
		}
		if err := checkLink(e.LinkTarget, len(parts)-1, e.Name, lim); err != nil {
			return "", err
		}
	}
	if err := checkSize(e, lim); err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return ".", nil
	}
	return strings.Join(parts, "/"), nil
}

// checkType refuses every entry type but regular, directory and symlink.
func checkType(e Entry) error {
	switch {
	case e.Hardlink:
		return reject(EntryHardlink, e.Name)
	case e.Mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0:
		return reject(EntrySetID, e.Name)
	}
	switch e.Mode.Type() {
	case 0, fs.ModeDir, fs.ModeSymlink:
		return nil
	case fs.ModeNamedPipe:
		return reject(EntryFIFO, e.Name)
	case fs.ModeSocket:
		return reject(EntrySocket, e.Name)
	case fs.ModeDevice, fs.ModeDevice | fs.ModeCharDevice:
		return reject(EntryDevice, e.Name)
	}
	return reject(EntryIrregular, e.Name)
}

// badBytes refuses NUL, control bytes and invalid UTF-8.
func badBytes(s, name string) error {
	if !utf8.ValidString(s) {
		return reject(PathUTF8, name)
	}
	for _, r := range s {
		// Cf covers the zero-width and bidi runes that HFS+ ignores in names
		// (".g<ZWNJ>it" names ".git") and that disguise a name on screen.
		if r < 0x20 || r == 0x7f || unicode.Is(unicode.Cf, r) {
			return reject(PathControl, name)
		}
	}
	return nil
}

// hasDrive is true for a Windows drive prefix. Any byte before the colon
// counts: "0:" is not a drive letter, but refusing it costs nothing.
func hasDrive(s string) bool { return len(s) >= 2 && s[1] == ':' }

// isDotGit is true for a component git treats as its own directory on some
// file system: ".git" in any letter case, with trailing dots or spaces (NTFS),
// or the NTFS short name "git~1". An artifact that carries one could plant a
// repository config (core.fsmonitor) that runs on the next git command there.
func isDotGit(c string) bool {
	c = strings.ToLower(strings.TrimRight(c, ". "))
	return c == ".git" || c == "git~1"
}

// splitName validates name and returns its components with "." dropped.
func splitName(name string, isDir bool, lim Limits) ([]string, error) {
	switch {
	case name == "":
		return nil, reject(PathEmpty, name)
	case len(name) > lim.MaxNameBytes:
		return nil, reject(PathTooLong, name)
	}
	if err := badBytes(name, name); err != nil {
		return nil, err
	}
	switch {
	case strings.Contains(name, `\`):
		return nil, reject(PathBackslash, name)
	case strings.HasPrefix(name, "/"):
		return nil, reject(PathAbsolute, name)
	case hasDrive(name):
		return nil, reject(PathDrive, name)
	}
	if strings.HasSuffix(name, "/") {
		if !isDir {
			return nil, reject(PathTrailingSlash, name)
		}
		name = strings.TrimSuffix(name, "/")
	}
	var parts []string
	for _, c := range strings.Split(name, "/") {
		switch {
		case c == ".":
		case c == "":
			return nil, reject(PathEmptyComponent, name)
		case c == "..":
			return nil, reject(PathTraversal, name)
		case strings.Trim(c, ".") == "":
			return nil, reject(PathDots, name)
		case len(c) > lim.MaxComponent:
			return nil, reject(PathTooLong, name)
		case !lim.AllowDotGit && isDotGit(c):
			return nil, reject(PathDotGit, name)
		default:
			parts = append(parts, c)
		}
	}
	if len(parts) > lim.MaxDepth {
		return nil, reject(PathTooDeep, name)
	}
	if len(parts) == 0 && !isDir {
		return nil, reject(PathEmpty, name)
	}
	return parts, nil
}

// checkLink validates a symlink target against the depth of the directory that
// holds the link (dirDepth components below the root). Leading ".." may climb
// no further than the root; a ".." after a normal component is refused.
func checkLink(target string, dirDepth int, name string, lim Limits) error {
	switch {
	case target == "":
		return reject(LinkEmpty, name)
	case strings.HasPrefix(target, "/"):
		return reject(LinkAbsolute, name)
	case strings.Contains(target, `\`) || hasDrive(target) || badBytes(target, name) != nil:
		return reject(LinkInvalid, name)
	}
	climbing := true
	for _, c := range strings.Split(target, "/") {
		switch {
		case c == "" || c == ".":
		case c == "..":
			if !climbing {
				return reject(LinkEscape, name)
			}
			if dirDepth--; dirDepth < 0 {
				return reject(LinkEscape, name)
			}
		case !lim.AllowDotGit && isDotGit(c):
			return reject(PathDotGit, name)
		default:
			climbing = false
		}
	}
	return nil
}

// foldKey maps a name to a case-insensitive identity: each rune becomes the
// smallest member of its simple-fold orbit.
func foldKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		f := r
		for c := unicode.SimpleFold(r); c != r; c = unicode.SimpleFold(c) {
			if c < f {
				f = c
			}
		}
		b.WriteRune(f)
	}
	return b.String()
}
