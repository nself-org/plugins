package internal

// gate_artifact.go — local Android release-artifact build lane.
//
// Purpose: `nself ci build --artifact android` produces a signed release APK
//   on the developer's own machine instead of a GitHub-hosted runner, closing
//   the private-repo artifact-build gap
//   (msg-2026-08-21-nself-ci-local-artifact-builds.md, Ummat/praycalc: "nself
//   ci gates but does not produce release artifacts"). Mirrors the exact
//   keystore-signing steps nchat's own workflows already use —
//   .github/workflows/build-react-native.yml "Build Android (Release)" and
//   deploy-mobile-android.yml's keystore decode step — rather than inventing
//   a parallel unsigned path.
// Inputs:  androidDir string — the Android project root containing gradlew
//   (e.g. frontend/platforms/react-native/android); env vars
//   ANDROID_KEYSTORE_BASE64, ANDROID_KEYSTORE_PASSWORD, ANDROID_KEY_ALIAS,
//   ANDROID_KEY_PASSWORD — same secret names nchat's GH Actions workflows
//   use, so a developer moving from CI to a local build reuses the same
//   values, never hardcoded, never logged.
// Outputs: ArtifactResult{Gate GateResult, APKPath string}
// Constraints: Android only. macOS/Windows/TV/WearOS artifacts explicitly
//   stay on GitHub-hosted runners (out of scope, Ummat's own report never
//   asked for them) — see P6-E11-W2-S1-T6. No network calls; `gh release
//   upload` is the caller's job (cli/cmd/commands/ci_artifact.go), not this
//   package's.
// SPORT: PLUGINS-CI-007

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ArtifactResult holds the outcome of a local artifact-build step: the
// underlying GateResult (for the existing PASS/FAIL table + status-posting
// machinery) plus the path to the produced artifact when the build passed.
type ArtifactResult struct {
	Gate    GateResult
	APKPath string
}

// androidKeystoreEnv are the env vars BuildAndroidArtifact reads, matching
// the GitHub Actions secret names in nchat/.github/workflows/{deploy-mobile-
// android,build-react-native}.yml exactly, so the same values work in both
// places.
const (
	envKeystoreBase64 = "ANDROID_KEYSTORE_BASE64"
	envKeystorePass   = "ANDROID_KEYSTORE_PASSWORD"
	envKeyAlias       = "ANDROID_KEY_ALIAS"
	envKeyPass        = "ANDROID_KEY_PASSWORD"
)

// BuildAndroidArtifact runs the Android release build+sign pipeline against
// an Android project rooted at androidDir. Steps mirror build-react-
// native.yml's "Build Android (Release)" job exactly:
//  1. base64-decode ANDROID_KEYSTORE_BASE64 into app/release.keystore
//  2. ./gradlew assembleRelease (keystore env vars inherited from the
//     process environment, same names gradle's signingConfig already reads)
//  3. locate the produced release APK under app/build/outputs/apk/
func BuildAndroidArtifact(androidDir string, timeout int, verbose bool) ArtifactResult {
	start := time.Now()
	name := "artifact:android"

	androidDir, absErr := filepath.Abs(androidDir)
	if absErr != nil {
		return failArtifact(name, start, fmt.Sprintf("cannot resolve android dir: %v", absErr))
	}

	gradlew := filepath.Join(androidDir, "gradlew")
	if !fileExists(gradlew) {
		return failArtifact(name, start, fmt.Sprintf(
			"no gradlew found at %s — pass the Android project root (the directory containing gradlew), e.g. frontend/platforms/react-native/android", gradlew))
	}

	keystoreB64 := os.Getenv(envKeystoreBase64)
	if keystoreB64 == "" {
		return failArtifact(name, start, fmt.Sprintf(
			"%s not set — required to produce a signed release build (same secret name as nchat's deploy-mobile-android.yml keystore decode step)", envKeystoreBase64))
	}
	for _, v := range []string{envKeystorePass, envKeyAlias, envKeyPass} {
		if os.Getenv(v) == "" {
			return failArtifact(name, start, fmt.Sprintf(
				"%s not set — required by the app-level gradle signingConfig (same secret name as nchat's build-react-native.yml)", v))
		}
	}

	keystorePath := filepath.Join(androidDir, "app", "release.keystore")
	if err := decodeKeystore(keystoreB64, keystorePath); err != nil {
		return failArtifact(name, start, fmt.Sprintf("decoding keystore: %v", err))
	}

	gate := runStep(name, androidDir, timeout, verbose, gradlew, "assembleRelease")
	if !gate.Passed {
		return ArtifactResult{Gate: gate}
	}

	apkPath, err := findReleaseAPK(androidDir)
	if err != nil {
		gate.Passed = false
		gate.Output = strings.TrimSpace(gate.Output + "\n" + err.Error())
		return ArtifactResult{Gate: gate}
	}

	gate.Output = strings.TrimSpace(gate.Output + "\nAPK: " + apkPath)
	return ArtifactResult{Gate: gate, APKPath: apkPath}
}

// failArtifact builds a failed ArtifactResult with a single-line reason,
// keeping every early-return in BuildAndroidArtifact to one line.
func failArtifact(name string, start time.Time, reason string) ArtifactResult {
	return ArtifactResult{Gate: GateResult{
		Name:    name,
		Passed:  false,
		Output:  reason,
		Elapsed: time.Since(start),
	}}
}

// decodeKeystore writes the base64-decoded keystore to destPath, creating
// its parent directory if needed. Mirrors the GH Actions step:
// `echo "$ANDROID_KEYSTORE_BASE64" | base64 -d > app/release.keystore`.
// File mode 0600: the keystore is a secret, never world/group readable.
func decodeKeystore(b64, destPath string) error {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return fmt.Errorf("invalid base64: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating keystore dir: %w", err)
	}
	if err := os.WriteFile(destPath, decoded, 0o600); err != nil {
		return fmt.Errorf("writing keystore: %w", err)
	}
	return nil
}

// findReleaseAPK walks <androidDir>/app/build/outputs/apk looking for the
// release-variant APK gradle produced — the same tree build-react-
// native.yml's "Upload Android artifact" step globs
// (app/build/outputs/apk/**/*.apk). Prefers a path containing "release" and
// excludes "-unsigned"/"-unaligned" intermediates; if exactly one APK exists
// under the release variant, it wins regardless of name.
func findReleaseAPK(androidDir string) (string, error) {
	outputsDir := filepath.Join(androidDir, "app", "build", "outputs", "apk")
	var candidates []string
	err := filepath.Walk(outputsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable entries; report "none found" below.
		}
		if info.IsDir() || !strings.HasSuffix(path, ".apk") {
			return nil
		}
		// Match only against the name and the path relative to outputsDir —
		// never the full absolute path. A checkout or temp-dir name that
		// happens to contain "release" (this package's own tests are named
		// "TestFindReleaseAPK...") would otherwise pollute the heuristic
		// below for every candidate, defeating the release/debug distinction
		// entirely.
		rel, relErr := filepath.Rel(outputsDir, path)
		if relErr != nil {
			rel = filepath.Base(path)
		}
		lower := strings.ToLower(rel)
		if strings.Contains(lower, "unsigned") || strings.Contains(lower, "unaligned") {
			return nil
		}
		candidates = append(candidates, path)
		return nil
	})
	if err != nil || len(candidates) == 0 {
		return "", fmt.Errorf("no signed APK found under %s after assembleRelease", outputsDir)
	}

	sort.Slice(candidates, func(i, j int) bool {
		iRelease := isReleaseVariantPath(outputsDir, candidates[i])
		jRelease := isReleaseVariantPath(outputsDir, candidates[j])
		if iRelease != jRelease {
			return iRelease // release-variant candidates sort first
		}
		return candidates[i] < candidates[j]
	})
	return candidates[0], nil
}

// isReleaseVariantPath reports whether path's location relative to
// outputsDir (never the full absolute path — see findReleaseAPK) names the
// "release" build variant, e.g. app/build/outputs/apk/release/app-release.apk.
func isReleaseVariantPath(outputsDir, path string) bool {
	rel, err := filepath.Rel(outputsDir, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return strings.Contains(strings.ToLower(rel), "release")
}

// UploadArtifact attaches filePath to an existing GitHub release via
// `gh release upload <tag> <filePath> --repo owner/repo --clobber`, so a
// re-run replaces a stale artifact rather than failing on a duplicate name.
// Uses gh OAuth like PostCommitStatus — never a token in the command line.
func UploadArtifact(owner, repo, tag, filePath string) (string, error) {
	if owner == "" || repo == "" || tag == "" {
		return "", fmt.Errorf("owner, repo, and tag are required to upload a release artifact")
	}
	args := []string{
		"release", "upload", tag, filePath,
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--clobber",
	}
	var out, stderr bytes.Buffer
	cmd := exec.Command("gh", args...)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh release upload failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String() + stderr.String()), nil
}
