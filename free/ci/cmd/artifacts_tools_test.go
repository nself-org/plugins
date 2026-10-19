package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

func syftArchive(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "syft", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// syftBinarySum is the sha256 of the `syft` entry inside a syftArchive.
func syftBinarySum(t *testing.T, archive []byte) string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	if _, err := tr.Next(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// toolsRig serves served and pins the checksum of pinned for the host platform.
func toolsRig(t *testing.T, served, pinned []byte) string {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(served) }))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(pinned)
	pin := sbom.Pin{Version: "1.54.1", Sources: []toolchain.Source{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: srv.URL + "/syft.tgz", SHA256: hex.EncodeToString(sum[:]), BinarySHA256: syftBinarySum(t, pinned)}}}
	oldPin, oldClient := toolsPin, toolsClient
	toolsPin, toolsClient = func() (sbom.Pin, error) { return pin, nil }, srv.Client()
	t.Cleanup(func() { toolsPin, toolsClient = oldPin, oldClient })
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_V15", "1")
	return home
}

func TestArtifactsToolsInstallPlacesThePinnedSyft(t *testing.T) {
	good := syftArchive(t, "#!/bin/sh\necho syft\n")
	home := toolsRig(t, good, good)
	code, out, errOut := runArt(t, "tools", "install")
	want := sbom.ToolPath(home, "1.54.1")
	if code != 0 || !strings.Contains(out, want) || !strings.Contains(out, "installed") {
		t.Fatalf("install: code %d out %q err %q", code, out, errOut)
	}
	if st, err := os.Stat(want); err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed file: %v %v", st, err)
	}
	if err := os.WriteFile(want, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runArt(t, "tools", "install", "--json")
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"data"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &env) != nil || env.Data.Path != want || len(env.Data.SHA256) != 64 {
		t.Fatalf("second install --json: code %d out %q", code, out)
	}
	if b, _ := os.ReadFile(want); !strings.Contains(string(b), "echo syft") {
		t.Fatalf("the reinstall did not repair the file: %q", b)
	}
}

func TestArtifactsToolsInstallRefusesOtherBytes(t *testing.T) {
	home := toolsRig(t, syftArchive(t, "evil"), syftArchive(t, "pinned"))
	code, out, errOut := runArt(t, "tools", "install", "--json")
	if code != 2 || !strings.Contains(out, "E718") || !strings.Contains(out, "sha256") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	if _, err := os.Stat(sbom.ToolPath(home, "1.54.1")); !os.IsNotExist(err) {
		t.Fatalf("a download with the wrong checksum was installed: %v", err)
	}
	code, _, errOut = runArt(t, "tools", "install")
	if code != 2 || !strings.Contains(errOut, "E718 tool_checksum") {
		t.Fatalf("plain output: code %d err %q", code, errOut)
	}
}

func TestArtifactsToolsUsage(t *testing.T) {
	for _, args := range [][]string{{"tools"}, {"tools", "upgrade"}, {"tools", "install", "extra"}} {
		if code, _, errOut := runArt(t, args...); code != 1 || !strings.Contains(errOut, "usage: nself ci artifacts tools install") {
			t.Errorf("%v: code %d err %q", args, code, errOut)
		}
	}
}
