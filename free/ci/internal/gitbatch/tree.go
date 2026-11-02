package gitbatch

// Purpose: `git ls-tree <rev> -- <path>` and `git rev-parse` / `cat-file -t` answers from the batch
// session, with the git-shaped results the call sites already parse.
// Inputs:  a revision (HEAD or a commit id) and a path relative to the session root (which may be a
//          subdirectory of the repository, as `git -C root ls-tree` treats it).
// Outputs: Entry (mode padded to six digits, type, object id) and whether the path exists.
// Constraints: a path git would treat specially (absolute, `..`, empty or `.` components, backslash) is
//          ErrUnavailable so the caller runs git; a missing path is found=false, never an error; a path
//          through a non-tree is found=false (ls-tree prints nothing for it); a tree that does not parse
//          is ErrMalformed (fail closed).

import (
	"context"
	"fmt"
	"strings"
)

// Entry is one ls-tree line.
type Entry struct {
	Mode string // "100644", "040000", "120000", "160000"
	Type string // blob, tree or commit (a submodule)
	OID  string
}

// TreeEntry is `git ls-tree <rev> -- <rel>` for one path.
func (s *Session) TreeEntry(ctx context.Context, rev, rel string) (Entry, bool, error) {
	parts, ok := splitRel(rel)
	if !ok {
		return Entry{}, false, fmt.Errorf("%w: path %q", ErrUnavailable, rel)
	}
	obj, body, err := s.Read(ctx, rev+":./")
	if err != nil {
		return Entry{}, false, err
	}
	if obj.Type != "tree" {
		return Entry{}, false, fmt.Errorf("%w: %s is a %s, not a tree", ErrMalformed, rev, obj.Type)
	}
	for i, want := range parts {
		e, found, err := findEntry(body, want, len(obj.OID)/2)
		if err != nil || !found {
			return Entry{}, false, err
		}
		if i == len(parts)-1 {
			return e, true, nil
		}
		if e.Type != "tree" {
			return Entry{}, false, nil
		}
		if obj, body, err = s.Read(ctx, e.OID); err != nil {
			return Entry{}, false, err
		}
		if obj.Type != "tree" {
			return Entry{}, false, fmt.Errorf("%w: %s is a %s, not a tree", ErrMalformed, e.OID, obj.Type)
		}
	}
	return Entry{}, false, nil
}

// splitRel cuts a relative path into components, refusing what ls-tree would read as something else.
func splitRel(rel string) ([]string, bool) {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.HasSuffix(rel, "/") || strings.Contains(rel, "\\") {
		return nil, false
	}
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, false
		}
	}
	return parts, true
}

// findEntry scans a raw tree (`<octal mode> SP <name> NUL <raw id>` records) for name.
func findEntry(tree []byte, name string, idLen int) (Entry, bool, error) {
	for i := 0; i < len(tree); {
		sp := indexByte(tree[i:], ' ')
		if sp < 1 {
			return Entry{}, false, fmt.Errorf("%w: tree record without a mode", ErrMalformed)
		}
		mode := string(tree[i : i+sp])
		i += sp + 1
		nul := indexByte(tree[i:], 0)
		if nul < 1 || i+nul+1+idLen > len(tree) {
			return Entry{}, false, fmt.Errorf("%w: tree record without a name or id", ErrMalformed)
		}
		entryName := string(tree[i : i+nul])
		i += nul + 1
		raw := tree[i : i+idLen]
		i += idLen
		if entryName != name {
			continue
		}
		typ, ok := modeType(mode)
		if !ok {
			return Entry{}, false, fmt.Errorf("%w: tree mode %q", ErrMalformed, mode)
		}
		return Entry{Mode: strings.Repeat("0", max(0, 6-len(mode))) + mode, Type: typ, OID: hexOf(raw)}, true, nil
	}
	return Entry{}, false, nil
}

func modeType(mode string) (string, bool) {
	switch {
	case mode == "40000":
		return "tree", true
	case mode == "160000":
		return "commit", true
	case mode == "120000" || mode == "100644" || mode == "100755":
		return "blob", true
	}
	return "", false
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func hexOf(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, digits[b>>4], digits[b&15])
	}
	return string(out)
}
