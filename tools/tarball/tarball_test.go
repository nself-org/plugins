package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture creates a small plugin tree under root/free/demo and returns root.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "free", "demo")
	long := strings.Repeat("d", 60) + "/" + strings.Repeat("f", 60) + ".txt"
	files := map[string]string{
		"plugin.json":   `{"name":"demo"}`,
		"b.txt":         "bee\n",
		"café.txt":      "non-ASCII name: PAX path record\n",
		"suid.sh":       "echo setuid\n",
		"a/z.sql":       "select 1;\n",
		"a/b/c.txt":     "deep\n",
		"install.sh":    "echo hi\n",
		long:            "long name\n",
		"Upper.md":      "sorted before lower\n",
		"cmd/demo/x.go": "package main\n",
	}
	for name, body := range files {
		fp := filepath.Join(p, name)
		if err := os.MkdirAll(filepath.Dir(fp), 0o775); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fp, []byte(body), 0o664); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"empty", "shared", "sticky"} {
		if err := os.MkdirAll(filepath.Join(p, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// setuid, setgid and sticky bits survive --mode='go-w,a+rX' in GNU tar.
	for name, mode := range map[string]os.FileMode{
		"install.sh": 0o775, "suid.sh": 0o775 | os.ModeSetuid,
		"shared": 0o775 | os.ModeSetgid, "sticky": 0o777 | os.ModeSticky,
	} {
		if err := os.Chmod(filepath.Join(p, name), mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("b.txt", filepath.Join(p, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(p, "b.txt"), filepath.Join(p, "b.hard")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	return root
}

func build(t *testing.T, root, out string) []byte {
	t.Helper()
	if err := DetTar(out, root, []string{"free/demo"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// readTar gunzips a .tar.gz and returns its headers in order.
func readTar(t *testing.T, gz []byte) (hs []*tar.Header, raw []byte) {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	if zr.Name != "" || !zr.ModTime.IsZero() {
		t.Fatalf("gzip header leaks name/mtime: %q %v", zr.Name, zr.ModTime)
	}
	if raw, err = io.ReadAll(zr); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return hs, raw
		}
		if err != nil {
			t.Fatal(err)
		}
		hs = append(hs, h)
	}
}

func names(hs []*tar.Header) string {
	var n []string
	for _, h := range hs {
		n = append(n, h.Name)
	}
	return strings.Join(n, "\n")
}

func TestDeterministicTarball(t *testing.T) {
	root := fixture(t)
	dist := t.TempDir()
	first := build(t, root, filepath.Join(dist, "one.tar.gz"))
	// Touch every file so on-disk mtimes differ between the two builds.
	later := time.Now().Add(time.Hour)
	if err := filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, later, later)
	}); err != nil {
		t.Fatal(err)
	}
	second := build(t, root, filepath.Join(dist, "two.tar.gz"))
	if !bytes.Equal(first, second) {
		t.Fatal("two builds of the same tree differ")
	}

	hs, raw := readTar(t, first)
	if len(raw)%recordSize != 0 {
		t.Fatalf("tar stream %d bytes is not a multiple of %d", len(raw), recordSize)
	}
	modes := map[string]*tar.Header{}
	for _, h := range hs {
		if h.ModTime.Unix() != 0 || h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" {
			t.Fatalf("%s: mtime=%v uid=%d gid=%d uname=%q", h.Name, h.ModTime, h.Uid, h.Gid, h.Uname)
		}
		modes[h.Name] = h
	}
	long := "free/demo/" + strings.Repeat("d", 60) + "/" + strings.Repeat("f", 60) + ".txt"
	want := []string{
		"free/demo/", "free/demo/Upper.md", "free/demo/a/", "free/demo/a/b/",
		"free/demo/a/b/c.txt", "free/demo/a/z.sql", "free/demo/b.hard", "free/demo/b.txt",
		"free/demo/café.txt", "free/demo/cmd/", "free/demo/cmd/demo/", "free/demo/cmd/demo/x.go",
		"free/demo/" + strings.Repeat("d", 60) + "/", long,
		"free/demo/empty/", "free/demo/install.sh", "free/demo/link", "free/demo/plugin.json",
		"free/demo/shared/", "free/demo/sticky/", "free/demo/suid.sh",
	}
	if got := names(hs); got != strings.Join(want, "\n") {
		t.Fatalf("entry order:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
	for name, m := range map[string]int64{
		"free/demo/b.txt": 0o644, "free/demo/install.sh": 0o755,
		"free/demo/empty/": 0o755, "free/demo/a/": 0o755, "free/demo/suid.sh": 0o4755,
		"free/demo/shared/": 0o2755, "free/demo/sticky/": 0o1755,
	} {
		if modes[name].Mode != m {
			t.Errorf("%s mode %o, want %o", name, modes[name].Mode, m)
		}
	}
	// The second name of one file is a type-1 link to the first, as in GNU tar.
	if h := modes["free/demo/b.txt"]; h.Typeflag != tar.TypeLink || h.Linkname != "free/demo/b.hard" || h.Size != 0 {
		t.Errorf("b.txt: type %q link %q size %d, want a hard link to b.hard", h.Typeflag, h.Linkname, h.Size)
	}
	if h := modes["free/demo/link"]; h.Typeflag != tar.TypeSymlink || h.Linkname != "b.txt" {
		t.Errorf("link: type %q target %q", h.Typeflag, h.Linkname)
	}
	if !bytes.Contains(raw, []byte("free/demo/PaxHeaders/café.txt")) {
		t.Error("non-ASCII name café.txt has no PAX path record, unlike GNU tar")
	}
	t.Run("gnu-tar-parity", func(t *testing.T) {
		bin := findGNUTar(t)
		gnuCompare(t, bin, root, raw, "-C", root, "free/demo")
		// Leading "/" and "../" are stripped from member names, as GNU tar does.
		abs := filepath.Join(root, "free", "demo")
		var buf bytes.Buffer
		if err := WriteTar(&buf, "ignored", []string{abs}); err != nil {
			t.Fatal(err)
		}
		gnuCompare(t, bin, root, buf.Bytes(), abs)
		buf.Reset()
		if err := WriteTar(&buf, filepath.Join(root, "free"), []string{"../free/demo"}); err != nil {
			t.Fatal(err)
		}
		gnuCompare(t, bin, root, buf.Bytes(), "-C", filepath.Join(root, "free"), "../free/demo")
	})
}

// findGNUTar returns a GNU tar binary (NSELF_GNU_TAR, then gtar, then tar)
// or skips the calling test.
func findGNUTar(t *testing.T) string {
	t.Helper()
	for _, c := range []string{os.Getenv("NSELF_GNU_TAR"), "gtar", "tar"} {
		if c == "" {
			continue
		}
		p, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		if out, err := exec.Command(p, "--version").Output(); err == nil && bytes.Contains(out, []byte("GNU tar")) {
			return p
		}
	}
	t.Skip("GNU tar not found: set NSELF_GNU_TAR or install gtar")
	return ""
}

// gnuCompare runs GNU tar with the flags of deterministic-tar.sh and the given
// tar arguments, and requires the exact bytes WriteTar produced.
func gnuCompare(t *testing.T, bin, dir string, raw []byte, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	full := append([]string{"--sort=name", "--mtime=UTC 1970-01-01",
		"--owner=0", "--group=0", "--numeric-owner", "--mode=go-w,a+rX",
		"--pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime", "-cf", "-"}, args...)
	cmd := exec.CommandContext(ctx, bin, full...)
	cmd.Dir = dir
	got, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("tar stream differs from GNU tar (%d vs %d bytes)", len(raw), len(got))
	}
}

func TestDetTarCommand(t *testing.T) {
	root := fixture(t)
	out := filepath.Join(t.TempDir(), "x.tar.gz")
	if err := run([]string{"det-tar", out, "-C", filepath.Join(root, "free"), "demo"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	hs, _ := readTar(t, b)
	if len(hs) == 0 || hs[0].Name != "demo/" || !strings.HasPrefix(names(hs), "demo/\ndemo/Upper.md") {
		t.Fatalf("-C was not applied; entries:\n%s", names(hs))
	}
	for _, bad := range [][]string{
		{"bogus"},
		{"det-tar", out},
		{"det-tar", out, "demo", "-C", "x"},
	} {
		if err := run(bad, io.Discard); err == nil {
			t.Fatalf("run(%q) accepted", bad)
		}
	}
}

func TestDetTarFailureLeavesNoFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.tar.gz")
	if err := DetTar(out, t.TempDir(), []string{"missing"}); err == nil {
		t.Fatal("missing member accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("partial output left behind: %v", err)
	}
}

func TestUnsupportedTypeAndOverflow(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("mkfifo", filepath.Join(root, "pipe")).Run(); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	err := WriteTar(io.Discard, root, []string{"pipe"})
	if err == nil || !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("FIFO must fail loudly, got %v", err)
	}
	if _, err := header("big", 0o644, 1<<33, '0', ""); err == nil {
		t.Fatal("8 GiB size must not truncate into a corrupt header")
	}
	if _, err := header("ok", 0o644, 1<<33-1, '0', ""); err != nil {
		t.Fatalf("largest ustar size rejected: %v", err)
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"free/demo": "free/demo", "/abs/p": "abs/p", "../x": "x", "a/../b": "b",
		"../../x/y": "x/y", "./a": "./a", "..": ".", "/": ".",
	} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}
