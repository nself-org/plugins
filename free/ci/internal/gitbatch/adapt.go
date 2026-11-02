package gitbatch

// Purpose: the one-line forms the call sites use in place of a git process: each returns what the git
// command would have decided, plus whether the batch session answered at all.
// Inputs:  the run's context (carrying the Set), the repository root and the object name.
// Outputs: (value, ok, used). used=false means "no session or ErrUnavailable: run the git command";
//          used=true, ok=false means git would have failed (missing, ambiguous, timeout, malformed).
// Constraints: nothing here guesses; every error other than ErrUnavailable is a failed read.

import (
	"context"
	"errors"
)

// RevParse is `git rev-parse <name>` for a revision expression: the object id it names.
func RevParse(ctx context.Context, root, name string) (oid string, ok, used bool) {
	s := For(ctx, root)
	if s == nil {
		return "", false, false
	}
	obj, err := s.Stat(ctx, name)
	if err != nil {
		return "", false, !errors.Is(err, ErrUnavailable)
	}
	return obj.OID, true, true
}

// ObjectType is `git cat-file -t <name>`.
func ObjectType(ctx context.Context, root, name string) (typ string, ok, used bool) {
	s := For(ctx, root)
	if s == nil {
		return "", false, false
	}
	obj, err := s.Stat(ctx, name)
	if err != nil {
		return "", false, !errors.Is(err, ErrUnavailable)
	}
	return obj.Type, true, true
}

// LsTree is `git -C root ls-tree <rev> -- <rel>`: ok is false when git would fail, found when it would
// print a line.
func LsTree(ctx context.Context, root, rev, rel string) (e Entry, found, ok, used bool) {
	s := For(ctx, root)
	if s == nil {
		return Entry{}, false, false, false
	}
	e, found, err := s.TreeEntry(ctx, rev, rel)
	if err != nil {
		return Entry{}, false, false, !errors.Is(err, ErrUnavailable)
	}
	return e, found, true, true
}

// ReadBlob is `git cat-file blob <oid>`: only a blob counts.
func ReadBlob(ctx context.Context, root, oid string) (data []byte, ok, used bool) {
	s := For(ctx, root)
	if s == nil {
		return nil, false, false
	}
	obj, data, err := s.Read(ctx, oid)
	if err != nil {
		return nil, false, !errors.Is(err, ErrUnavailable)
	}
	return data, obj.Type == "blob", true
}
