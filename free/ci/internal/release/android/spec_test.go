package android

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJobShape(t *testing.T) {
	j := Job("app-dir", []string{"b", "a"}, 90*time.Second)
	if j.Kind != "release" || j.Isolation != "process" || j.Workdir != "app-dir" || j.Timeout != "90s" || *j.Required != true || *j.Idempotent != false {
		t.Fatalf("job %+v", j)
	}
	if strings.Join(j.Secrets, " ") != "release/ANDROID_KEYSTORE_BASE64 release/ANDROID_KEYSTORE_PASSWORD release/ANDROID_KEY_ALIAS release/ANDROID_KEY_PASSWORD" {
		t.Fatalf("secrets %v", j.Secrets)
	}
	if len(j.Artifacts) != 2 || j.Artifacts[0].Path != "app/build/outputs/apk/release/*.apk" || j.Artifacts[0].Kind != "package" || j.Artifacts[0].Name != "android" {
		t.Fatalf("artifacts %+v", j.Artifacts)
	}
	if j.Artifacts[1].Kind != "sbom" || !strings.HasPrefix(j.Artifacts[1].Path, SBOMDir) {
		t.Fatalf("sbom decl %+v", j.Artifacts[1])
	}
	if j.Run[0] != "sh" || j.Run[1] != "-c" || len(j.Env) != 0 {
		t.Fatalf("run %v env %v", j.Run, j.Env)
	}
	if got := Job(".", nil, 0).Timeout; got != "1s" {
		t.Fatalf("a zero timeout became %q", got)
	}
}

// runScript runs the job script in dir with env, the way the executor does.
func runScript(t *testing.T, dir string, env ...string) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	return cmd.Run()
}

func fakeGradlew(t *testing.T, dir string, exit string) {
	t.Helper()
	body := "#!/bin/sh\ntest \"$1\" = assembleRelease || exit 9\ncp app/release.keystore seen.bin\nexit " + exit + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The script decodes the keystore (wrapped base64 too), runs gradle on it and removes it, pass or fail.
func TestScriptDecodesAndRemovesKeystore(t *testing.T) {
	keystore := []byte("fake-keystore-bytes\x00\x01\x02")
	b64 := base64.StdEncoding.EncodeToString(keystore)
	wrapped := b64[:8] + "\n" + b64[8:] + "\n"
	for _, tc := range []struct {
		name, value, exit string
		fail              bool
	}{{"plain", b64, "0", false}, {"wrapped", wrapped, "0", false}, {"gradle fails", b64, "3", true}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			fakeGradlew(t, dir, tc.exit)
			err := runScript(t, dir, "ANDROID_KEYSTORE_BASE64="+tc.value)
			if (err != nil) != tc.fail {
				t.Fatalf("err %v, want failure %v", err, tc.fail)
			}
			seen, rerr := os.ReadFile(filepath.Join(dir, "seen.bin"))
			if rerr != nil || string(seen) != string(keystore) {
				t.Fatalf("gradle saw %q (%v)", seen, rerr)
			}
			if _, serr := os.Stat(filepath.Join(dir, KeystorePath)); serr == nil {
				t.Fatal("the keystore was left behind")
			}
		})
	}
	dir := t.TempDir()
	fakeGradlew(t, dir, "0")
	if err := runScript(t, dir, "ANDROID_KEYSTORE_BASE64="); err == nil {
		t.Fatal("an empty keystore variable built anyway")
	}
	if _, err := os.Stat(filepath.Join(dir, "seen.bin")); err == nil {
		t.Fatal("gradle ran without a keystore")
	}
}
