// Package detect discovers CI inputs from a bounded, read-only fs.FS snapshot.
package detect

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Confidence describes how directly the evidence proves a fact.
type Confidence string

const (
	Certain Confidence = "certain"
	Unknown Confidence = "unknown"
)

// Fact is one discovered input. Path always names the evidence in the snapshot.
type Fact struct {
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Confidence Confidence `json:"confidence"`
	SHA256     string     `json:"sha256,omitempty"`
	Reason     string     `json:"reason,omitempty"`
}

// Snapshot is a bounded index of files; it never executes project code.
type Snapshot struct {
	FS    fs.FS
	files map[string]struct{}
}

const maxDepth = 6
const maxFiles = 50000
const maxRead = 1 << 20

var ignored = map[string]bool{
	"vendor": true, "node_modules": true, ".git": true, "dist": true,
	"build": true, "target": true, ".nself": true,
}

// NewSnapshot indexes at most 50,000 files at depth six, in fs.WalkDir order.
func NewSnapshot(source fs.FS) (Snapshot, error) {
	s := Snapshot{FS: source, files: make(map[string]struct{})}
	err := fs.WalkDir(source, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		depth := strings.Count(p, "/") + 1
		if d.IsDir() {
			if ignored[d.Name()] || depth > maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if depth > maxDepth {
			return nil
		}
		if len(s.files) >= maxFiles {
			return fs.ErrInvalid
		}
		s.files[p] = struct{}{}
		return nil
	})
	return s, err
}

// Paths returns a sorted copy of paths with the given base name.
func (s Snapshot) Paths(base string) []string {
	var paths []string
	for p := range s.files {
		if path.Base(p) == base {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

func (s Snapshot) has(p string) bool { _, ok := s.files[p]; return ok }

func (s Snapshot) read(p string) ([]byte, error) {
	if !s.has(p) {
		return nil, fs.ErrNotExist
	}
	f, err := s.FS.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxRead+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxRead {
		return nil, fs.ErrInvalid
	}
	return b, nil
}

func fact(kind, name, p string) Fact {
	return Fact{Kind: kind, Name: name, Path: p, Confidence: Certain}
}

func unknown(kind, p, reason string) Fact {
	return Fact{Kind: kind, Name: "unknown", Path: p, Confidence: Unknown, Reason: reason}
}

func (s Snapshot) lockDigest(p string) (string, error) {
	f, err := s.FS.Open(p)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, (64<<20)+1))
	if err != nil || n > 64<<20 {
		return "", fs.ErrInvalid
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
