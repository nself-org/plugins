package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pluginTree writes plugins/demo (binaryName demo, a cmd package, a symlink).
func pluginTree(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	p := filepath.Join(root, "free", "demo")
	for name, body := range map[string]string{
		"plugin.json":        `{"name":"demo","binaryName":"nself-demo"}`,
		"go.mod":             "module demo\n\ngo 1.21\n",
		"cmd/demo/main.go":   "package main\n\nfunc main() {}\n",
		"migrations/001.sql": "select 1;\n",
	} {
		fp := filepath.Join(p, name)
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fp, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("plugin.json", filepath.Join(p, "alias.json")); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestBuildTarballs runs the whole build verb: source layout, checksum files,
// a platform tarball for a plugin holding a symlink, and no stage leftovers.
func TestBuildTarballs(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	old := platforms
	platforms = []string{"linux-amd64"}
	t.Cleanup(func() { platforms = old })
	root := pluginTree(t)
	dist := filepath.Join(t.TempDir(), "dist")
	var log bytes.Buffer
	if err := run([]string{"build", "-plugins-dir", filepath.Join(root, "free"), "-dist", dist, "1.2.3"}, &log); err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	src, err := os.ReadFile(filepath.Join(dist, "demo-1.2.3.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	hs, _ := readTar(t, src)
	// An absolute plugins dir loses its leading "/" in member names, as with GNU tar.
	prefix := strings.TrimPrefix(filepath.ToSlash(filepath.Join(root, "free", "demo")), "/")
	if len(hs) == 0 || hs[0].Name != prefix+"/" {
		t.Fatalf("source tarball first member %q, want %q", names(hs), prefix+"/")
	}
	plat, err := os.ReadFile(filepath.Join(dist, "demo-1.2.3-linux-amd64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	ph, _ := readTar(t, plat)
	got := map[string]*tar.Header{}
	for _, h := range ph {
		got[h.Name] = h
	}
	if got["demo/nself-demo"] == nil || got["demo/nself-demo"].Mode&0o111 == 0 {
		t.Fatalf("platform tarball lacks an executable demo/nself-demo:\n%s", names(ph))
	}
	if h := got["demo/alias.json"]; h == nil || h.Typeflag != tar.TypeSymlink || h.Linkname != "plugin.json" {
		t.Fatalf("symlink not carried into the platform tarball:\n%s", names(ph))
	}
	for _, f := range []string{"demo-1.2.3.tar.gz", "demo-1.2.3-linux-amd64.tar.gz"} {
		sum, err := os.ReadFile(filepath.Join(dist, f+".sha256"))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(dist, f))
		if want := fmt.Sprintf("%x  %s\n", sha256.Sum256(b), f); string(sum) != want {
			t.Errorf("%s.sha256 = %q, want %q", f, sum, want)
		}
	}
	left, _ := filepath.Glob(filepath.Join(dist, "stage-*"))
	if len(left) != 0 {
		t.Fatalf("stage directories left behind: %v", left)
	}
}

// TestBuildSourceLayout pins the default layout (free/<name>/...) and the
// rejection of versions that would escape the dist directory.
func TestBuildSourceLayout(t *testing.T) {
	root := pluginTree(t)
	t.Chdir(root)
	dist := t.TempDir()
	if err := run([]string{"build", "-source-only", "-dist", dist, "9.9.9"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dist, "demo-9.9.9.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	hs, _ := readTar(t, b)
	if len(hs) == 0 || hs[0].Name != "free/demo/" {
		t.Fatalf("default layout wrong:\n%s", names(hs))
	}
	if err := run([]string{"build", "-source-only", "-dist", dist, "../evil"}, io.Discard); err == nil {
		t.Fatal("path-like version accepted")
	}
}
