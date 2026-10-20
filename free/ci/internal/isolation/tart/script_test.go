package tart

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

func TestScriptCarriesEnvSecretsAndExec(t *testing.T) {
	const planted = "planted-secret-value-123"
	got, err := buildScript([]string{"make", "it's"}, "work/a1/sub", map[string]string{"B": "x y", "A": "1"}, map[string]string{"TOKEN": planted})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	want := "set -e\nexport A='1'\nexport B='x y'\nexport TOKEN='" + planted + "'\ncd -- 'work/a1/sub'\nexec 'make' 'it'\\''s'\n"
	if s != want {
		t.Fatalf("script:\n%s\nwant:\n%s", s, want)
	}
}

func TestScriptRefusesBadInput(t *testing.T) {
	if _, err := buildScript([]string{"x"}, "w", map[string]string{"bad-name": "v"}, nil); err == nil {
		t.Fatal("bad env name")
	}
	if _, err := buildScript([]string{"x"}, "w", map[string]string{"A": "a\x00b"}, nil); err == nil {
		t.Fatal("NUL value")
	}
	if _, err := buildScript([]string{"x\x00"}, "w", nil, nil); err == nil {
		t.Fatal("NUL arg")
	}
	if _, err := buildScript([]string{"x"}, "w", nil, map[string]string{"lower": "v"}); err == nil {
		t.Fatal("secret name")
	}
	for _, wd := range []string{"../x", "/abs", "a/../../b"} {
		if _, err := workdirIn("work/a", wd); err == nil {
			t.Fatalf("workdir %q escaped", wd)
		}
	}
	if got, _ := workdirIn("work/a", "sub/dir"); got != "work/a/sub/dir" {
		t.Fatal(got)
	}
}

func TestScriptRunsInAShellWithHostileValues(t *testing.T) {
	hostile := "a'b\"c$(echo pwned)`echo pwned`\n;rm -rf /"
	dir := t.TempDir()
	script, err := buildScript([]string{"sh", "-c", `printf '%s' "$V"`}, ".", map[string]string{"V": hostile}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmd := osexec.Command("/bin/bash", "-s")
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(script)
	out, err := cmd.Output()
	if err != nil || string(out) != hostile {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestLazyReaderBuildsOnFirstReadAndZeroes(t *testing.T) {
	calls := 0
	r := &lazyReader{build: func() ([]byte, error) { calls++; return []byte("secret-bytes"), nil }}
	if calls != 0 {
		t.Fatal("built too early")
	}
	b, _ := io.ReadAll(r)
	if string(b) != "secret-bytes" || calls != 1 {
		t.Fatal(string(b), calls)
	}
	if !bytes.Equal(r.data, make([]byte, len(r.data))) {
		t.Fatal("the buffer must be zeroed after the last read")
	}
	bad := &lazyReader{build: func() ([]byte, error) { return nil, errors.New("boom") }}
	if _, err := io.ReadAll(bad); err == nil {
		t.Fatal("build error must surface")
	}
}

func TestTarWorkspaceRoundTripSkipsSpecialFiles(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "d"), 0o755)
	os.WriteFile(filepath.Join(src, "d", "f.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\n"), 0o755)
	os.Symlink("d/f.txt", filepath.Join(src, "link"))
	l, err := net.Listen("unix", filepath.Join(src, "sock"))
	if err == nil {
		defer l.Close()
	}
	root, err := os.OpenRoot(src)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var buf bytes.Buffer
	if err := tarWorkspace(root, &buf); err != nil {
		t.Fatal(err)
	}
	names := map[string]byte{}
	tr := tar.NewReader(&buf)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names[h.Name] = h.Typeflag
		if h.Uid != 0 || h.Uname != "" {
			t.Fatalf("owner leaked: %+v", h)
		}
	}
	if names["d/f.txt"] != tar.TypeReg || names["link"] != tar.TypeSymlink || names["d/"] != tar.TypeDir || names["run.sh"] != tar.TypeReg {
		t.Fatalf("entries %v", names)
	}
	if _, ok := names["sock"]; ok {
		t.Fatal("a socket must be skipped")
	}
}

func mkTar(entries ...tar.Header) string {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for _, h := range entries {
		h := h
		tw.WriteHeader(&h)
		if h.Size > 0 {
			tw.Write(bytes.Repeat([]byte("x"), int(h.Size)))
		}
	}
	tw.Close()
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestExtractArtifactsIsHardened(t *testing.T) {
	cases := []struct {
		name string
		h    tar.Header
		ok   bool
	}{
		{"declared file", tar.Header{Name: "dist/a.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3}, true},
		{"declared dir", tar.Header{Name: "dist/", Typeflag: tar.TypeDir, Mode: 0o755}, true},
		{"traversal", tar.Header{Name: "dist/../../evil", Typeflag: tar.TypeReg, Size: 1}, false},
		{"absolute", tar.Header{Name: "/etc/evil", Typeflag: tar.TypeReg, Size: 1}, false},
		{"undeclared", tar.Header{Name: "other/a.txt", Typeflag: tar.TypeReg, Size: 1}, false},
		{"prefix lookalike", tar.Header{Name: "distant/a.txt", Typeflag: tar.TypeReg, Size: 1}, false},
		{"symlink", tar.Header{Name: "dist/l", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, false},
		{"hardlink", tar.Header{Name: "dist/h", Typeflag: tar.TypeLink, Linkname: "dist/a"}, false},
		{"device", tar.Header{Name: "dist/n", Typeflag: tar.TypeChar}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			root, _ := os.OpenRoot(dir)
			defer root.Close()
			err := extractArtifacts(strings.NewReader(mkTar(tc.h)), root, []string{"dist"}, 1<<20)
			if (err == nil) != tc.ok {
				t.Fatalf("err %v", err)
			}
		})
	}
	dir := t.TempDir()
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	if err := extractArtifacts(strings.NewReader(mkTar(tar.Header{Name: "dist/big", Typeflag: tar.TypeReg, Mode: 0o644, Size: 100})), root, []string{"dist"}, 50); err == nil {
		t.Fatal("size cap")
	}
}

func TestExtractCannotEscapeThroughAnExistingSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	os.Symlink(outside, filepath.Join(dir, "dist")) // the host workspace already holds a link out
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	err := extractArtifacts(strings.NewReader(mkTar(tar.Header{Name: "dist/a.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3})), root, []string{"dist"}, 1<<20)
	if _, e := os.Stat(filepath.Join(outside, "a.txt")); e == nil || err == nil {
		t.Fatalf("a write escaped the workspace root: err=%v", err)
	}
}

func TestCleanArtifactPaths(t *testing.T) {
	if got, err := cleanArtifactPaths([]string{"dist/", "./out/x.zip"}); err != nil || got[0] != "dist" || got[1] != "out/x.zip" {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"", ".", "..", "../x", "/abs", "a/*", "a\nb"} {
		if _, err := cleanArtifactPaths([]string{bad}); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestFramedPayloadIgnoresNoiseAndReportsStatus(t *testing.T) {
	in := "ssh: warning\n" + beginMark + "\nQUJD\nREVG\n" + endMark + " 0\ntrailing\n"
	p, status := framedPayload(strings.NewReader(in))
	b, _ := io.ReadAll(p)
	code, err := status()
	if string(b) != "QUJDREVG" || code != 0 || err != nil {
		t.Fatalf("%q %d %v", b, code, err)
	}
	p, status = framedPayload(strings.NewReader(beginMark + "\nQUJD\n"))
	_, _ = io.ReadAll(p)
	if _, err := status(); err == nil {
		t.Fatal("a stream with no end marker is incomplete")
	}
}

func TestBuildFacts(t *testing.T) {
	host := func() (netscope.Facts, error) {
		return netscope.Facts{HostAddrs: []netip.Addr{netip.MustParseAddr("192.168.1.2")}}, nil
	}
	res := fakeResolver{"deploy.example": {netip.MustParseAddr("203.0.113.9")}}
	f, err := BuildFacts(host, func(context.Context) (netscope.Exclusions, error) {
		return netscope.Exclusions{Addresses: []string{"5.75.235.42"}, Prefixes: []string{"10.9.0.0/16"}, Hosts: []string{"deploy.example", "198.51.100.1", "missing.example"}}, nil
	}, res)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range f.Exclusion {
		got[p.String()] = true
	}
	for _, want := range []string{"5.75.235.42/32", "10.9.0.0/16", "203.0.113.9/32", "198.51.100.1/32"} {
		if !got[want] {
			t.Fatalf("exclusion lacks %s: %v", want, f.Exclusion)
		}
	}
	if len(f.HostAddrs) != 1 || len(f.ExclusionHosts) != 3 {
		t.Fatalf("%+v", f)
	}
	bad := func(ex netscope.Exclusions) error {
		_, err := BuildFacts(host, func(context.Context) (netscope.Exclusions, error) { return ex, nil }, res)(context.Background())
		return err
	}
	for _, ex := range []netscope.Exclusions{{Addresses: []string{"nope"}}, {Prefixes: []string{"10.0.0.0/99"}}, {Hosts: []string{""}}, {Hosts: []string{"servfail.example"}}} {
		if !isScopeErr(bad(ex)) {
			t.Fatalf("%+v accepted", ex)
		}
	}
	if _, err := BuildFacts(host, func(context.Context) (netscope.Exclusions, error) {
		return netscope.Exclusions{}, errors.New("unreadable")
	}, res)(context.Background()); !isScopeErr(err) {
		t.Fatalf("unreadable set: %v", err)
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	if host == "missing.example" {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return nil, errors.New("servfail")
}
