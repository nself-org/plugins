// Regression tests for the local Android artifact-build lane
// (P6-E11-W2-S1-T6, msg-2026-08-21-nself-ci-local-artifact-builds.md).
package internal

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeKeystore_WritesDecodedBytesWithRestrictivePermissions(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "app", "release.keystore")
	payload := []byte("not-a-real-keystore-just-bytes")
	b64 := base64.StdEncoding.EncodeToString(payload)

	if err := decodeKeystore(b64, dest); err != nil {
		t.Fatalf("decodeKeystore: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading decoded keystore: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("decoded content mismatch: got %q want %q", got, payload)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("keystore file mode = %o, want 0600 (it is a secret)", info.Mode().Perm())
	}
}

func TestDecodeKeystore_RejectsInvalidBase64(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "app", "release.keystore")
	if err := decodeKeystore("not-valid-base64!!!", dest); err == nil {
		t.Fatal("expected an error for invalid base64, got nil")
	}
}

func TestFindReleaseAPK_PrefersReleasePathAndExcludesUnsigned(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "app", "build", "outputs", "apk", "release")
	debugDir := filepath.Join(dir, "app", "build", "outputs", "apk", "debug")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		t.Fatal(err)
	}

	unsigned := filepath.Join(releaseDir, "app-release-unsigned.apk")
	signed := filepath.Join(releaseDir, "app-release.apk")
	debugAPK := filepath.Join(debugDir, "app-debug.apk")
	for _, p := range []string{unsigned, signed, debugAPK} {
		if err := os.WriteFile(p, []byte("apk"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := findReleaseAPK(dir)
	if err != nil {
		t.Fatalf("findReleaseAPK: %v", err)
	}
	if got != signed {
		t.Errorf("findReleaseAPK() = %q, want the signed release APK %q (never the -unsigned intermediate or a debug build)", got, signed)
	}
}

func TestFindReleaseAPK_NoAPKsIsAnError(t *testing.T) {
	dir := t.TempDir()
	if _, err := findReleaseAPK(dir); err == nil {
		t.Fatal("expected an error when no APK was produced, got nil")
	}
}

func TestBuildAndroidArtifact_MissingGradlewFailsFast(t *testing.T) {
	dir := t.TempDir()
	result := BuildAndroidArtifact(dir, 5, false)
	if result.Gate.Passed {
		t.Fatal("expected failure when gradlew is absent, got Passed=true")
	}
}

func TestBuildAndroidArtifact_MissingKeystoreEnvFailsFast(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envKeystoreBase64, "")
	result := BuildAndroidArtifact(dir, 5, false)
	if result.Gate.Passed {
		t.Fatal("expected failure when ANDROID_KEYSTORE_BASE64 is unset, got Passed=true")
	}
}
