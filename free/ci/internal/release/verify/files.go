package verify

// Purpose: read a release directory safely and check SHA256SUMS and every
// file digest (Epic CACHE D16 `checksum`).
// Inputs:  the release directory and the decoded manifest.
// Outputs: a fileSet of the digests found, or a Failure.
// Constraints: a path from a document is joined under the directory only after
// its parts are checked (no empty, "." or ".." part, no backslash, no NUL), and
// no part may be a symbolic link, so a hostile release cannot make the
// verifier read outside it. Files are hashed by streaming. A file the
// directory holds but SHA256SUMS does not list, or the reverse, fails.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxEnvelope = 8 << 20
	maxSums     = 16 << 20
	maxEvidence = 64 << 20
)

type fileInfo struct {
	sha  string
	size int64
}

// fileSet holds the digests the checksum step computed.
type fileSet struct {
	byPath      map[string]fileInfo
	missingSBOM []string // listed in SHA256SUMS under sbom/ but absent: reported as sbom_link
}

type sumLine struct{ sha, path string }

// safePath returns the OS path of rel under dir, or an error when rel is not a
// plain relative slash path or a part of it is a symbolic link.
func safePath(dir, rel string) (string, error) {
	if rel == "" || strings.ContainsAny(rel, "\\\x00") {
		return "", fmt.Errorf("path %q is not allowed", rel)
	}
	cur := dir
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("path %q is not allowed", rel)
		}
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%q passes through a symbolic link", rel)
		}
	}
	return cur, nil
}

// open opens the regular file rel under dir.
func open(dir, rel string) (*os.File, os.FileInfo, error) {
	p, err := safePath(dir, rel)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err == nil && !fi.Mode().IsRegular() {
		err = fmt.Errorf("%q is not a regular file", rel)
	}
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, fi, nil
}

// readFile reads rel under dir, refusing more than max bytes.
func readFile(dir, rel string, max int64) ([]byte, error) {
	f, fi, err := open(dir, rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if fi.Size() > max {
		return nil, fmt.Errorf("%q is larger than %d bytes", rel, max)
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err == nil && int64(len(b)) > max {
		err = fmt.Errorf("%q is larger than %d bytes", rel, max)
	}
	return b, err
}

// digest hashes rel under dir.
func digest(dir, rel string) (fileInfo, error) {
	f, _, err := open(dir, rel)
	if err != nil {
		return fileInfo{}, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return fileInfo{sha: hex.EncodeToString(h.Sum(nil)), size: n}, err
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// parseSums reads SHA256SUMS: `<64 hex>  <path>` per line, each ended by a
// newline, paths strictly ascending.
func parseSums(b []byte) ([]sumLine, error) {
	if len(b) == 0 || b[len(b)-1] != '\n' {
		return nil, errors.New("SHA256SUMS is empty or does not end with a newline")
	}
	var out []sumLine
	for i, line := range strings.Split(string(b[:len(b)-1]), "\n") {
		if len(line) < 67 || line[64:66] != "  " || !isHex(line[:64]) {
			return nil, fmt.Errorf("SHA256SUMS line %d is not `<sha256>  <path>`", i+1)
		}
		l := sumLine{sha: line[:64], path: line[66:]}
		if i > 0 && l.path <= out[i-1].path {
			return nil, fmt.Errorf("SHA256SUMS line %d is not in ascending path order", i+1)
		}
		out = append(out, l)
	}
	return out, nil
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// declared lists the files the manifest names, with the digest it states
// (empty for evidence.json, which has its own step).
func (r *run) declared() map[string]string {
	m := r.man
	want := map[string]string{m.Evidence.Path: "", m.Provenance.Path: m.Provenance.SHA256}
	for _, a := range m.Artifacts {
		want[a.Path] = a.SHA256
		want[a.SBOM.CycloneDX.Path] = a.SBOM.CycloneDX.SHA256
		want[a.SBOM.SPDX.Path] = a.SBOM.SPDX.SHA256
	}
	return want
}

// checksums checks SHA256SUMS against the manifest, the directory and the files.
func (r *run) checksums() *Failure {
	b, err := readFile(r.dir, r.man.Checksums.Path, maxSums)
	if err != nil {
		return fail(ReasonChecksum, "cannot read %s: %v", r.man.Checksums.Path, err)
	}
	if sum(b) != r.man.Checksums.SHA256 {
		return fail(ReasonChecksum, "%s does not match the digest in the signed manifest", r.man.Checksums.Path)
	}
	lines, err := parseSums(b)
	if err != nil {
		return fail(ReasonChecksum, "%v", err)
	}
	want := r.declared()
	listed := make(map[string]bool, len(lines))
	for _, l := range lines {
		if _, ok := want[l.path]; !ok {
			return fail(ReasonChecksum, "%s lists %q, which the manifest does not declare", r.man.Checksums.Path, l.path)
		}
		listed[l.path] = true
	}
	for _, p := range sortedKeys(want) {
		if !listed[p] {
			return fail(ReasonChecksum, "%s does not cover %q", r.man.Checksums.Path, p)
		}
	}
	r.files = &fileSet{byPath: make(map[string]fileInfo, len(lines))}
	for _, l := range lines {
		info, err := digest(r.dir, l.path)
		switch {
		case errors.Is(err, fs.ErrNotExist) && strings.HasPrefix(l.path, "sbom/"):
			r.files.missingSBOM = append(r.files.missingSBOM, l.path)
			continue
		case err != nil:
			return fail(ReasonChecksum, "cannot read %q: %v", l.path, err)
		case info.sha != l.sha:
			return fail(ReasonChecksum, "%q does not match its line in %s", l.path, r.man.Checksums.Path)
		}
		if exp := want[l.path]; exp != "" && info.sha != exp {
			return fail(ReasonChecksum, "%q does not match the digest in the signed manifest", l.path)
		}
		r.files.byPath[l.path] = info
	}
	for _, a := range r.man.Artifacts {
		if info, ok := r.files.byPath[a.Path]; ok && info.size != a.Size {
			return fail(ReasonChecksum, "%q is %d bytes, the manifest says %d", a.Path, info.size, a.Size)
		}
	}
	return r.unlisted(listed)
}

// unlisted refuses a release directory that holds a file SHA256SUMS does not
// cover, or anything that is not a regular file.
func (r *run) unlisted(listed map[string]bool) *Failure {
	var bad *Failure
	err := filepath.WalkDir(r.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(r.dir, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		switch {
		case !d.Type().IsRegular():
			bad = fail(ReasonChecksum, "%q is not a regular file", rel)
		case rel != ManifestFile && rel != r.man.Checksums.Path && !listed[rel]:
			bad = fail(ReasonChecksum, "%q is not listed in %s", rel, r.man.Checksums.Path)
		}
		if bad != nil {
			return fs.SkipAll
		}
		return nil
	})
	if bad != nil {
		return bad
	}
	if err != nil {
		return fail(ReasonChecksum, "cannot list the release directory: %v", err)
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
