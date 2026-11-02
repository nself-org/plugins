package specs

// Purpose: the restore sets each file's mode and time through its descriptor
// and writes the files of a directory through one pinned handle (P7-CI-46).
// Constraints: archive order is the packer's (a directory's files together),
// but a crafted order must restore the same tree; modes and whole-second times
// match the archive for small files (parallel writers) and for large ones.

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

func settleArchive(t *testing.T, names []string, at time.Time, big string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	seen := map[string]bool{}
	for _, n := range names {
		if d := filepath.ToSlash(filepath.Dir(n)); d != "." && !seen[d] {
			seen[d] = true
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: d + "/", Mode: 0o755, ModTime: at}); err != nil {
				t.Fatal(err)
			}
		}
		body := "body of " + n
		if n == big {
			body = strings.Repeat("x", smallFile+1)
		}
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o640, Size: int64(len(body)), ModTime: at}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUnpackSettlesModeAndTimeThroughTheDescriptor(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	var names []string
	for i := range 40 {
		names = append(names, fmt.Sprintf("d%02d/f%d", i%7, i)) // directories interleave: each reopens its handle
	}
	names = append(names, "top", "d00/large")
	for _, other := range []bool{false, true} {
		base, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = base.Close() })
		arc := settleArchive(t, names, at, "d00/large")
		if _, err := Unpack(bytes.NewReader(arc), base, "out", authz.Limits{}, UnpackOptions{OtherRead: other}); err != nil {
			t.Fatalf("other=%v: %v", other, err)
		}
		want := os.FileMode(0o640)
		if other {
			want = 0o644
		}
		for _, n := range names {
			p := filepath.Join(base.Name(), "out", n)
			info, err := os.Stat(p)
			if err != nil {
				t.Fatalf("other=%v: %v", other, err)
			}
			if info.Mode().Perm() != want {
				t.Errorf("other=%v %s: mode %v, want %v", other, n, info.Mode().Perm(), want)
			}
			if !info.ModTime().Equal(at) {
				t.Errorf("other=%v %s: mtime %v, want %v", other, n, info.ModTime(), at)
			}
			if n != "d00/large" {
				if got := getFile(t, p); got != "body of "+n {
					t.Errorf("other=%v %s: content %q", other, n, got)
				}
			}
		}
	}
}
