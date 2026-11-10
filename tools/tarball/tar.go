package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// blockSize is the tar block; recordSize is GNU tar's default 20-block record.
const (
	blockSize  = 512
	recordSize = 20 * blockSize
)

// normMode applies GNU tar --mode='go-w,a+rX' to a permission set.
func normMode(perm os.FileMode, dir bool) int64 {
	m := int64(perm.Perm()) &^ 0o022
	m |= 0o444
	if dir || m&0o111 != 0 {
		m |= 0o111
	}
	return m
}

// octal writes v as zero-padded octal of width len(dst)-1 plus a NUL. It
// fails when v does not fit, instead of truncating into a corrupt header.
func octal(dst []byte, v int64) error {
	s := strconv.FormatInt(v, 8)
	if v < 0 || len(s) > len(dst)-1 {
		return fmt.Errorf("value %d does not fit a %d-byte octal tar field (ustar caps sizes at 8 GiB)", v, len(dst))
	}
	copy(dst, strings.Repeat("0", len(dst)-1-len(s))+s)
	dst[len(dst)-1] = 0
	return nil
}

// header builds one POSIX ustar block with uid/gid 0, no owner names, epoch
// mtime. devmajor/devminor (offsets 329-344) stay NUL, as GNU tar writes them
// for these member types; the GNU parity subtest proves it byte for byte.
func header(name string, mode int64, size int64, typ byte, link string) ([]byte, error) {
	b := make([]byte, blockSize)
	copy(b[0:100], name)
	for _, f := range []struct {
		dst []byte
		v   int64
	}{{b[100:108], mode}, {b[108:116], 0}, {b[116:124], 0}, {b[124:136], size}, {b[136:148], 0}} {
		if err := octal(f.dst, f.v); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	b[156] = typ
	copy(b[157:257], link)
	copy(b[257:265], "ustar\x0000")
	for i := 148; i < 156; i++ {
		b[i] = ' '
	}
	var sum int64
	for _, c := range b {
		sum += int64(c)
	}
	copy(b[148:156], fmt.Sprintf("%06o\x00 ", sum))
	return b, nil
}

// seenFile is a regular file already written (for hard-link detection).
type seenFile struct {
	name string
	fi   os.FileInfo
}

// tarWriter emits a GNU tar stream and counts bytes for record padding.
type tarWriter struct {
	w    io.Writer
	n    int64
	seen map[int64][]seenFile // regular files by size
}

func (t *tarWriter) write(p []byte) error {
	n, err := t.w.Write(p)
	t.n += int64(n)
	return err
}

func (t *tarWriter) pad() error { return t.padTo(blockSize) }

// padTo zero-fills to the next multiple of unit.
func (t *tarWriter) padTo(unit int64) error {
	if r := t.n % unit; r != 0 {
		return t.write(make([]byte, unit-r))
	}
	return nil
}

// paxRecord formats one "<len> key=value\n" record, where len counts itself.
func paxRecord(key, val string) string {
	body := " " + key + "=" + val + "\n"
	n := len(body) + 1
	for len(strconv.Itoa(n))+len(body) != n {
		n = len(strconv.Itoa(n)) + len(body)
	}
	return strconv.Itoa(n) + body
}

// put writes one header block.
func (t *tarWriter) put(name string, mode, size int64, typ byte, link string) error {
	h, err := header(name, mode, size, typ, link)
	if err != nil {
		return err
	}
	return t.write(h)
}

// paxHeader writes a PAX extended header for name, named
// "%d/PaxHeaders/%f" as deterministic-tar.sh's --pax-option sets.
func (t *tarWriter) paxHeader(name, records string) error {
	base := strings.TrimSuffix(name, "/")
	xname := path.Dir(base) + "/PaxHeaders/" + path.Base(base)
	if err := t.put(xname, 0o644, int64(len(records)), 'x', ""); err != nil {
		return err
	}
	if err := t.write([]byte(records)); err != nil {
		return err
	}
	return t.pad()
}

// hardLink returns the member a regular file is a hard link to, or records it.
func (t *tarWriter) hardLink(name string, fi os.FileInfo) (string, bool) {
	for _, s := range t.seen[fi.Size()] {
		if os.SameFile(s.fi, fi) {
			return s.name, true
		}
	}
	t.seen[fi.Size()] = append(t.seen[fi.Size()], seenFile{name, fi})
	return "", false
}

// entry writes one member: a header (plus PAX records for long names) and the
// body of a regular file. A second name of one file becomes a type-1 link.
func (t *tarWriter) entry(name, src string, fi os.FileInfo) error {
	var typ byte = '0'
	var link string
	var size int64
	switch {
	case fi.IsDir():
		typ, name = '5', name+"/"
	case fi.Mode()&os.ModeSymlink != 0:
		typ = '2'
		l, err := os.Readlink(src)
		if err != nil {
			return err
		}
		link = l
	case fi.Mode().IsRegular():
		size = fi.Size()
		if first, ok := t.hardLink(name, fi); ok {
			typ, link, size = '1', first, 0
		}
	default:
		return fmt.Errorf("%s: unsupported file type %v (only regular files, directories and symlinks can be archived)", src, fi.Mode().Type())
	}
	var recs string
	if len(link) > 100 {
		recs += paxRecord("linkpath", link)
	}
	if len(name) > 100 {
		recs += paxRecord("path", name)
	}
	if recs != "" {
		if err := t.paxHeader(name, recs); err != nil {
			return err
		}
	}
	if err := t.put(name, normMode(fi.Mode(), fi.IsDir()), size, typ, link); err != nil {
		return err
	}
	if typ != '0' { // dirs, symlinks and hard links carry no body
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	if n, err := io.Copy(t, f); err != nil || n != size {
		return fmt.Errorf("%s: archived %d of %d bytes (file changed or unreadable: %v)", src, n, size, err)
	}
	return t.pad()
}

// Write lets io.Copy stream file bodies through the byte counter.
func (t *tarWriter) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	t.n += int64(n)
	return n, err
}

// safeName mirrors GNU tar: drop everything up to the last "..", then leading
// slashes; empty becomes ".".
func safeName(p string) string {
	parts := strings.Split(p, "/")
	start := 0
	for i, c := range parts {
		if c == ".." {
			start = i + 1
		}
	}
	if out := strings.TrimLeft(strings.Join(parts[start:], "/"), "/"); out != "" {
		return out
	}
	return "."
}

// walk archives the file at src under member name and, for a directory, its
// children in byte-sorted order, depth first, like GNU tar --sort=name.
func (t *tarWriter) walk(src, name string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := t.entry(name, src, fi); err != nil {
		return err
	}
	if !fi.IsDir() {
		return nil
	}
	kids, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, k := range kids {
		if err := t.walk(filepath.Join(src, k.Name()), name+"/"+k.Name()); err != nil {
			return err
		}
	}
	return nil
}

// WriteTar writes the deterministic tar stream for members (relative to base,
// or absolute) to w, ending with two zero blocks padded to a full record.
func WriteTar(w io.Writer, base string, members []string) error {
	t := &tarWriter{w: w, seen: map[int64][]seenFile{}}
	for _, m := range members {
		if m = strings.TrimSuffix(m, "/"); m == "" {
			return errors.New("empty member path")
		}
		src := m
		if !filepath.IsAbs(m) {
			src = filepath.Join(base, m)
		}
		if err := t.walk(src, safeName(m)); err != nil {
			return err
		}
	}
	if err := t.write(make([]byte, 2*blockSize)); err != nil {
		return err
	}
	return t.padTo(recordSize)
}

// DetTar writes out as a gzip (no name, zero mtime, best compression) of the
// deterministic tar of members. The tar stream matches deterministic-tar.sh
// byte for byte; the gzip layer does not (Go's deflate is not `gzip -n -9`),
// so the .tar.gz and its sha256 differ from the script's. A failed build
// leaves no partial file behind.
func DetTar(out, base string, members []string) (err error) {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(out) // no partial archive
		}
	}()
	bw := bufio.NewWriter(f)
	zw, err := gzip.NewWriterLevel(bw, gzip.BestCompression)
	if err != nil {
		f.Close()
		return err
	}
	zw.OS = 3
	err = WriteTar(zw, base, members)
	for _, closeFn := range []func() error{zw.Close, bw.Flush, f.Close} {
		if cerr := closeFn(); err == nil {
			err = cerr
		}
	}
	return err
}
