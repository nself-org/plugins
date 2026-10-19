package specs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

type tent struct {
	name, link, body string
	typ              byte
	mode             int64
	size             int64 // overrides len(body) when > 0
}

// tarGz builds a gzip level 1 tar of ents (the shape Pack writes).
func tarGz(t testing.TB, ents ...tent) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	tw := tar.NewWriter(gz)
	for _, e := range ents {
		h := &tar.Header{Name: e.name, Linkname: e.link, Typeflag: e.typ, Mode: e.mode}
		if e.typ == tar.TypeXGlobalHeader {
			if err := tw.WriteHeader(&tar.Header{Typeflag: e.typ, PAXRecords: map[string]string{"comment": "x"}}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
			if e.size > 0 {
				h.Size = e.size
			}
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			body := []byte(e.body)
			if int64(len(body)) < h.Size {
				body = append(body, make([]byte, h.Size-int64(len(body)))...)
			}
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tree lists every path under dir with its type, mode and content or target.
func tree(t testing.TB, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		info, _ := d.Info()
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			out = append(out, rel+" -> "+l)
		case d.IsDir():
			out = append(out, rel+"/")
		default:
			b, _ := os.ReadFile(p)
			out = append(out, rel+" "+info.Mode().Perm().String()+" "+string(b))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func TestPackUnpackRoundTrip(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	putFile(t, src+"/a/b/c.txt", "deep")
	putFile(t, src+"/top.bin", strings.Repeat("0123456789", 5000))
	putFile(t, src+"/ro/frozen.txt", "readonly")
	if err := os.Mkdir(src+"/empty", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src+"/ro/frozen.txt", 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src+"/ro", 0o555); err != nil { // a Go module cache holds read-only directories
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveTree(src) })
	for _, l := range [][2]string{{"a/b/c.txt", "link-in"}, {"../a/b/c.txt", "a/up"}, {"/etc/passwd", "abs"}, {"../../outside", "escape"}} {
		if err := os.Symlink(l[0], filepath.Join(src, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Mkfifo(src+"/fifo", 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	st, err := Pack(src, gz, cacheLimits())
	if err != nil || gz.Close() != nil {
		t.Fatalf("Pack: %v", err)
	}
	if st.Files != 3 || st.Links != 2 || st.Skipped != 3 {
		t.Fatalf("stats %+v: want 3 files, 2 in-tree links, and the fifo and both escaping links skipped", st)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if _, err := Unpack(&buf, dst, cacheLimits()); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	t.Cleanup(func() { _ = RemoveTree(dst) })
	got := tree(t, dst)
	want := []string{"a/", "a/b/", "a/b/c.txt -rw-r--r-- deep", "a/up -> ../a/b/c.txt", "empty/", "link-in -> a/b/c.txt", "ro/", "ro/frozen.txt -r--r--r-- readonly", "top.bin -rw-r--r-- " + strings.Repeat("0123456789", 5000)}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("restored tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if err := RemoveTree(dst); err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("tree survived RemoveTree")
	}
}

func TestUnpackRefusesCraftedArchives(t *testing.T) {
	bomb := tent{name: "zeros.bin", size: 8 << 20}
	cases := []struct {
		name   string
		ents   []tent
		reason authz.PathReason
	}{
		{"parent traversal", []tent{{name: "../x", body: "evil"}}, authz.PathTraversal},
		{"nested traversal", []tent{{name: "a/../../x", body: "evil"}}, authz.PathTraversal},
		{"absolute path", []tent{{name: "/tmp/x", body: "evil"}}, authz.PathAbsolute},
		{"absolute symlink", []tent{{name: "l", typ: tar.TypeSymlink, link: "/etc"}}, authz.LinkAbsolute},
		{"symlink out", []tent{{name: "l", typ: tar.TypeSymlink, link: "../../out"}}, authz.LinkEscape},
		{"write through a symlink", []tent{{name: "d", typ: tar.TypeSymlink, link: "."}, {name: "d/x", body: "evil"}}, authz.PathThroughSymlink},
		{"hardlink out", []tent{{name: "h", typ: tar.TypeLink, link: "../secret"}}, authz.EntryHardlink},
		{"hardlink in", []tent{{name: "f", body: "x"}, {name: "h", typ: tar.TypeLink, link: "f"}}, authz.EntryHardlink},
		{"device", []tent{{name: "dev", typ: tar.TypeChar}}, authz.EntryDevice},
		{"fifo", []tent{{name: "p", typ: tar.TypeFifo}}, authz.EntryFIFO},
		{"setuid", []tent{{name: "s", body: "x", mode: 0o4755}}, authz.EntrySetID},
		{"pax global header", []tent{{name: "g", typ: tar.TypeXGlobalHeader}}, authz.EntryIrregular},
		{"duplicate file", []tent{{name: "f", body: "1"}, {name: "f", body: "2"}}, authz.CollisionDuplicate},
		{"case collision", []tent{{name: "README", body: "1"}, {name: "readme", body: "2"}}, authz.CollisionFold},
		{"ratio bomb", []tent{bomb}, authz.SizeRatio},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "dir")
			before := tree(t, parent)
			_, err := Unpack(bytes.NewReader(tarGz(t, c.ents...)), dir, cacheLimits())
			var pe *authz.PathError
			if !errors.As(err, &pe) || pe.Reason != c.reason {
				t.Fatalf("err %v, want reason %s", err, c.reason)
			}
			for _, p := range tree(t, parent) {
				if !strings.HasPrefix(p, "dir") && !contains(before, p) {
					t.Fatalf("a path outside the attempt dir was written: %s", p)
				}
			}
			if _, err := os.Lstat(filepath.Join(parent, "x")); err == nil {
				t.Fatal("x escaped")
			}
		})
	}
}

func TestUnpackBoundsTheDecompressedStream(t *testing.T) {
	lim := cacheLimits()
	lim.MaxTotalBytes = 1 << 20
	body := bytes.Repeat([]byte("ab"), 200<<10) // 400 KiB per file, compresses far below the ratio floor concerns
	data := tarGz(t, tent{name: "a", body: string(body)}, tent{name: "b", body: string(body)}, tent{name: "c", body: string(body)})
	_, err := Unpack(bytes.NewReader(data), t.TempDir()+"/d", lim)
	var pe *authz.PathError
	if !errors.As(err, &pe) || pe.Reason != authz.SizeTotal {
		t.Fatalf("err %v", err)
	}
	if _, err := Unpack(strings.NewReader("not gzip at all"), t.TempDir()+"/d", cacheLimits()); err == nil {
		t.Fatal("a non-gzip stream was accepted")
	}
	if _, err := Unpack(bytes.NewReader(tarGz(t, tent{name: "a", body: "x"})[:30]), t.TempDir()+"/d", cacheLimits()); err == nil {
		t.Fatal("a truncated archive was accepted")
	}
}

func TestUnpackRefusesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	putFile(t, dir+"/f", "mine")
	if _, err := Unpack(bytes.NewReader(tarGz(t, tent{name: "f", body: "theirs"})), dir, cacheLimits()); err == nil || getFile(t, dir+"/f") != "mine" {
		t.Fatalf("an existing file was replaced (err %v)", err)
	}
}

func TestPackStopsAtTheLimits(t *testing.T) {
	src := t.TempDir()
	putFile(t, src+"/a", "1")
	putFile(t, src+"/b", "2")
	lim := cacheLimits()
	lim.MaxEntries = 1
	if _, err := Pack(src, io.Discard, lim); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err %v", err)
	}
}
