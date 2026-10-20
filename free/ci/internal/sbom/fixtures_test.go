//go:build sbomtool

package sbom_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildGoFixture compiles testdata/gobin (one third-party dependency) into dir/gobin.
func buildGoFixture(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", filepath.Join(dir, "gobin"), "./internal/sbom/testdata/gobin")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the Go fixture: %v\n%s", err, out)
	}
}

const pnpmLock = `lockfileVersion: '9.0'

importers:

  .:
    dependencies:
      left-pad:
        specifier: 1.3.0
        version: 1.3.0

packages:

  left-pad@1.3.0:
    resolution: {integrity: sha512-XI5MPzVNApjAyhQzphX8BkmKsKUxD4LdyK24iZeQGinBN9yTQT3bFlCBy/aVx2HrNcqQGsdot8ghrjyrvMCoEA==}

snapshots:

  left-pad@1.3.0: {}
`

// writePnpmTarball writes a package tarball (what `pnpm pack` makes) with one direct dependency.
func writePnpmTarball(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("package/package.json", `{"name":"fixture-pkg","version":"1.0.0","dependencies":{"left-pad":"1.3.0"}}`)
	add("package/pnpm-lock.yaml", pnpmLock)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

const apkDB = "C:Q1aaaaaaaaaaaaaaaaaaaaaaaaaaa=\nP:busybox\nV:1.36.1-r0\nA:x86_64\nT:fixture toolbox\nL:GPL-2.0-only\n\n"

// writeOCIArchive writes a one-layer OCI image archive whose rootfs carries an apk database.
func writeOCIArchive(t *testing.T, path string) {
	t.Helper()
	var layer bytes.Buffer
	lw := tar.NewWriter(&layer)
	for name, body := range map[string]string{"lib/apk/db/installed": apkDB, "etc/os-release": "ID=alpine\nVERSION_ID=3.19.0\n"} {
		if err := lw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := lw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := lw.Close(); err != nil {
		t.Fatal(err)
	}
	digest := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	mustJSON := func(v any) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	desc := func(mt string, b []byte) map[string]any {
		return map[string]any{"mediaType": mt, "digest": "sha256:" + digest(b), "size": len(b)}
	}
	config := mustJSON(map[string]any{"architecture": "amd64", "os": "linux", "config": map[string]any{},
		"rootfs": map[string]any{"type": "layers", "diff_ids": []string{"sha256:" + digest(layer.Bytes())}}})
	manifest := mustJSON(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
		"config": desc("application/vnd.oci.image.config.v1+json", config),
		"layers": []any{desc("application/vnd.oci.image.layer.v1.tar", layer.Bytes())}})
	index := mustJSON(map[string]any{"schemaVersion": 2, "manifests": []any{desc("application/vnd.oci.image.manifest.v1+json", manifest)}})
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	for name, body := range map[string][]byte{
		"oci-layout":                            []byte(`{"imageLayoutVersion":"1.0.0"}`),
		"index.json":                            index,
		"blobs/sha256/" + digest(config):        config,
		"blobs/sha256/" + digest(manifest):      manifest,
		"blobs/sha256/" + digest(layer.Bytes()): layer.Bytes(),
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
