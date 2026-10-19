//go:build integration_android

package android

// Purpose: the operator-laptop leg of P7-CACHE-15 (Epic CACHE D18, R-DX-12): a bare
// macOS laptop, no Tart, no other node, NSELF_V15=1, ANDROID_KEYSTORE_* exported by
// the operator, a real Android SDK. The lane builds a real signed APK, inventories
// it with the pinned syft, publishes the release and the release verifies.
// Inputs (all prerequisites; a missing one is t.Fatal, never t.Skip):
//   NSELF_ANDROID_FIXTURE   absolute path of an Android project (gradlew and app/ whose release
//                           signingConfig reads the four ANDROID_* variables)
//   ANDROID_KEYSTORE_BASE64, ANDROID_KEYSTORE_PASSWORD, ANDROID_KEY_ALIAS, ANDROID_KEY_PASSWORD
//   ANDROID_HOME or ANDROID_SDK_ROOT, JAVA_HOME of a JDK the project's Gradle supports
//   `nself` on PATH with `config secrets resolve` (the V15 cli)
//   the pinned syft: `nself ci artifacts tools install` under NSELF_CI_HOME (default ~/.nself/ci)
// Constraints: no network of its own (gradle fetches its own dependencies); the exported
// values are the sinks' planted values: none may appear in any file of the run.

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// copyProject copies the fixture project, leaving out build output, caches and any keystore.
func copyProject(t *testing.T, from, to string) {
	t.Helper()
	if err := os.MkdirAll(to, 0o755); err != nil {
		t.Fatal(err)
	}
	skip := map[string]bool{"build": true, ".gradle": true, "local.properties": true, "release.keystore": true, ".git": true}
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if rel == "." {
			return nil
		}
		if skip[d.Name()] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(dst, b, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if os.Getenv(n) == "" {
			t.Fatalf("%s is not set: the laptop leg needs the operator's environment", n)
		}
	}
}

func TestAndroidLaneBareLaptop(t *testing.T) {
	needIntegration(t)
	t.Logf("host %s/%s", runtime.GOOS, runtime.GOARCH)
	mustEnv(t, "NSELF_ANDROID_FIXTURE", "ANDROID_KEYSTORE_BASE64", "ANDROID_KEYSTORE_PASSWORD", "ANDROID_KEY_ALIAS", "ANDROID_KEY_PASSWORD", "JAVA_HOME")
	sdk := firstNonEmpty(os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT"))
	if sdk == "" {
		t.Fatal("ANDROID_HOME or ANDROID_SDK_ROOT is not set: the laptop leg needs a real Android SDK")
	}
	t.Setenv("ANDROID_HOME", sdk)
	signers, _ := filepath.Glob(filepath.Join(sdk, "build-tools", "*", "apksigner"))
	if len(signers) == 0 {
		t.Fatalf("no build-tools/*/apksigner under %s", sdk)
	}
	sort.Strings(signers)
	apksigner := signers[len(signers)-1]
	if out, err := exec.Command("nself", "config", "secrets", "resolve", "--help").CombinedOutput(); err != nil || !strings.Contains(string(out), "resolve") {
		t.Fatalf("`nself config secrets resolve` is not available (the V15 cli is required): %v\n%s", err, out)
	}
	pin, err := sbom.EmbeddedPin()
	if err != nil {
		t.Fatal(err)
	}
	home, err := sbom.ResolveHome("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(sbom.ToolPath(home, pin.Version)); err != nil {
		t.Fatalf("the pinned syft is not installed (%v): run `%s`", err, sbom.InstallCommand)
	}

	p := &rig{t: t}
	p.setup(func(repo string) {
		copyProject(t, os.Getenv("NSELF_ANDROID_FIXTURE"), filepath.Join(repo, "android"))
		writeFiles(t, repo, map[string]string{".gitignore": "app/build/\napp/release.keystore\n.gradle/\nlocal.properties\n"})
	})
	p.sbomStep = &SBOMStep{Home: home, Pin: pin}
	res, st, out := p.run(run.SecretsRuntime{Mode: secrets.ModePersonalLocal})
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s notes %v\n%s", res.Exit, res.Summary.Result, res.Summary.Notes, out)
	}
	row, ok, err := st.ReleaseFor(context.Background(), res.RunID)
	if err != nil || !ok || row.State != store.ReleasePublished {
		t.Fatalf("release record %+v %v %v", row, ok, err)
	}
	d := p.deps(st)
	exp, err := publish.Export(context.Background(), d, res.RunID, filepath.Join(t.TempDir(), "export"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	apk := filepath.Join(exp.Dir, "artifacts", ArtifactName, "app-release.apk")
	if sig, err := exec.Command(apksigner, "verify", "--print-certs", apk).CombinedOutput(); err != nil || !strings.Contains(string(sig), "Signer #1 certificate") {
		t.Fatalf("apksigner verify %s: %v\n%s", apk, err, sig)
	}
	info, err := d.Signer.PublicKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v := p.verifyDir(exp.Dir, info); !v.Verified || v.Exit != 0 {
		t.Fatalf("verify: %+v", v)
	}
	p.cliVerify(exp.Dir, info)
	for _, f := range []string{"sbom/android/app-release.apk.cdx.json", "sbom/android/app-release.apk.spdx.json", "provenance.intoto.dsse.json"} {
		if _, err := os.Stat(filepath.Join(exp.Dir, f)); err != nil {
			t.Errorf("the release lacks %s", f)
		}
	}
	operator := []string{os.Getenv("ANDROID_KEYSTORE_BASE64"), os.Getenv("ANDROID_KEYSTORE_PASSWORD"), os.Getenv("ANDROID_KEY_ALIAS"), os.Getenv("ANDROID_KEY_PASSWORD")}
	if hits := sinkHits(t, operator, map[string]string{"run output": out}, p.home, exp.Dir, p.repo); len(hits) != 0 {
		t.Fatalf("the operator's keystore values reached sinks: %v", hits)
	}
	t.Logf("APK %s\nrelease %s\nrun %s key %s", apk, exp.Dir, res.RunID, info.KeyID)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
