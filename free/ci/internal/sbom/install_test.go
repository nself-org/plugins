package sbom_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

type tarEntry struct {
	name string
	body string
	typ  byte
	link string
}

func makeTarGz(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o755, Linkname: e.link}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
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

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// installRig serves body and returns options whose pin names wantSum for the host platform.
func installRig(t *testing.T, body []byte, wantSum string, status int) (sbom.InstallOptions, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	pin := sbom.Pin{Version: pinVersion, Sources: []toolchain.Source{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: srv.URL + "/syft.tar.gz", SHA256: wantSum}}}
	return sbom.InstallOptions{Home: t.TempDir(), Pin: pin, Client: srv.Client()}, &hits
}

func toolsDirEntries(t *testing.T, home string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(filepath.Join(home, "tools"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func TestInstallPlacesTheBinaryAfterTheChecksum(t *testing.T) {
	archive := makeTarGz(t, tarEntry{name: "LICENSE", body: "x"}, tarEntry{name: "syft", body: "#!/bin/sh\necho pinned\n"})
	o, hits := installRig(t, archive, sha(archive), 0)
	res, err := sbom.Install(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	want := sbom.ToolPath(o.Home, pinVersion)
	if res.Path != want || res.SHA256 != sha(archive) {
		t.Fatalf("res = %+v", res)
	}
	st, err := os.Stat(want)
	if err != nil || st.Mode().Perm() != 0o755 {
		t.Fatalf("installed file: %v %v", st, err)
	}
	if b, _ := os.ReadFile(want); string(b) != "#!/bin/sh\necho pinned\n" {
		t.Fatalf("content = %q", b)
	}
	if got := toolsDirEntries(t, o.Home); len(got) != 1 {
		t.Fatalf("temp files left behind: %v", got)
	}
	// a second install downloads and checks again, and repairs a damaged file
	if err := os.WriteFile(want, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := sbom.Install(context.Background(), o); err != nil || hits.Load() != 2 {
		t.Fatalf("second install: %v hits=%d", err, hits.Load())
	}
	if b, _ := os.ReadFile(want); string(b) != "#!/bin/sh\necho pinned\n" {
		t.Fatalf("the reinstall did not replace the file: %q", b)
	}
}

func TestInstallRefusesASymlinkDestination(t *testing.T) {
	archive := makeTarGz(t, tarEntry{name: "syft", body: "good"})
	o, _ := installRig(t, archive, sha(archive), 0)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := sbom.ToolPath(o.Home, pinVersion)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := sbom.Install(context.Background(), o); !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), "not a plain file") {
		t.Fatalf("symlink destination: %v", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("the symlink target was overwritten: %q", b)
	}
}

func TestInstallRefusesAnHTTPSDowngradeRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("never served")) }))
	defer plain.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/syft.tar.gz", http.StatusFound)
	}))
	defer redirect.Close()
	pin := sbom.Pin{Version: pinVersion, Sources: []toolchain.Source{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: redirect.URL + "/syft.tar.gz", SHA256: strings.Repeat("a", 64)}}}
	_, err := sbom.Install(context.Background(), sbom.InstallOptions{Home: t.TempDir(), Pin: pin, Client: redirect.Client()})
	if !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), "not https") {
		t.Fatalf("downgrade redirect: %v", err)
	}
}

func TestInstallRefusesOtherBytes(t *testing.T) {
	good := makeTarGz(t, tarEntry{name: "syft", body: "good"})
	evil := makeTarGz(t, tarEntry{name: "syft", body: "evil"})
	o, _ := installRig(t, evil, sha(good), 0)
	_, err := sbom.Install(context.Background(), o)
	if !sbom.IsReason(err, sbom.ReasonToolChecksum) {
		t.Fatalf("got %v, want E718 tool_checksum", err)
	}
	if _, statErr := os.Stat(sbom.ToolPath(o.Home, pinVersion)); !os.IsNotExist(statErr) {
		t.Fatalf("a refused download was installed: %v", statErr)
	}
	if got := toolsDirEntries(t, o.Home); len(got) != 0 {
		t.Fatalf("temp files left behind: %v", got)
	}
}

func TestInstallRefusesBadArchivesAndServers(t *testing.T) {
	ctx := context.Background()
	noSyft := makeTarGz(t, tarEntry{name: "README", body: "x"})
	linkSyft := makeTarGz(t, tarEntry{name: "syft", typ: tar.TypeSymlink, link: "/bin/sh"})
	nested := makeTarGz(t, tarEntry{name: "sub/syft", body: "x"})
	traversal := makeTarGz(t, tarEntry{name: "../syft", body: "x"})
	notGzip := []byte("plain bytes, not an archive")
	for name, body := range map[string][]byte{"no syft": noSyft, "symlink named syft": linkSyft, "nested syft": nested, "traversal": traversal, "not gzip": notGzip} {
		o, _ := installRig(t, body, sha(body), 0) // checksum right, content wrong
		if _, err := sbom.Install(ctx, o); err == nil {
			t.Errorf("%s: installed", name)
		} else if !sbom.IsReason(err, sbom.ReasonToolChecksum) {
			t.Errorf("%s: got %v", name, err)
		}
		if got := toolsDirEntries(t, o.Home); len(got) != 0 {
			t.Errorf("%s: left %v", name, got)
		}
	}
	o, _ := installRig(t, nil, strings.Repeat("0", 64), http.StatusNotFound)
	if _, err := sbom.Install(ctx, o); !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), "404") {
		t.Fatalf("HTTP 404: %v", err)
	}
}

func TestInstallRefusesUnusableOptions(t *testing.T) {
	ctx := context.Background()
	pin := sbom.Pin{Version: pinVersion, Sources: []toolchain.Source{{OS: "linux", Arch: "amd64", URL: "http://example.invalid/syft.tgz", SHA256: strings.Repeat("a", 64)}}}
	httpsPin := sbom.Pin{Version: pinVersion, Sources: []toolchain.Source{{OS: "linux", Arch: "amd64", URL: "https://example.invalid/syft.tgz", SHA256: strings.Repeat("a", 64)}}}
	if _, err := sbom.Install(ctx, sbom.InstallOptions{Home: t.TempDir(), Pin: pin, OS: "plan9", Arch: "mips"}); !sbom.IsReason(err, sbom.ReasonToolMissing) {
		t.Fatalf("no source for the platform: %v", err)
	}
	if _, err := sbom.Install(ctx, sbom.InstallOptions{Home: t.TempDir(), Pin: pin, OS: "linux", Arch: "amd64"}); !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), "https") {
		t.Fatalf("plain http source: %v", err)
	}
	if _, err := sbom.Install(ctx, sbom.InstallOptions{Home: "relative/home", Pin: httpsPin, OS: "linux", Arch: "amd64"}); !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative home: %v", err)
	}
	bad := sbom.Pin{Version: "../../x", Sources: httpsPin.Sources}
	if _, err := sbom.Install(ctx, sbom.InstallOptions{Home: t.TempDir(), Pin: bad, OS: "linux", Arch: "amd64"}); !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), "plain version") {
		t.Fatalf("path-like version: %v", err)
	}
}
