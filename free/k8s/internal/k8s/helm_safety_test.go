package k8s

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// releaseJSON is helm's `status --output json` for a release whose config and
// manifest carry secrets, as with real helm.
const releaseJSON = `{"name":"rel","namespace":"ns1","version":3,
 "info":{"status":"deployed","last_deployed":"2026-10-10T00:00:00Z","description":"Upgrade complete"},
 "chart":{"metadata":{"name":"nself","version":"0.4.2"}},
 "config":{"license":{"key":"needle-licence-two"},"secrets":{"hasura":{"ADMIN":"needle-value-one"}}},
 "manifest":"kind: Secret\nstringData:\n  ADMIN: needle-manifest-three\n"}`

// TestStatusPrintsOnlySummary (M1): status prints name, namespace, revision,
// chart version and status; helm's config and rendered manifest never reach
// the output.
func TestStatusPrintsOnlySummary(t *testing.T) {
	out := fakeHelm(t)
	writeFile(t, filepath.Join(out, "status.json"), releaseJSON, 0o600)
	got, err := Status(context.Background(), "rel", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{needleValue, needleLicence, "needle-manifest-three", "config", "manifest", "Secret"} {
		if strings.Contains(got, needle) {
			t.Errorf("status output leaks %q: %s", needle, got)
		}
	}
	for _, want := range []string{`"name":"rel"`, `"namespace":"ns1"`, `"version":3`, `"chart_version":"0.4.2"`, `"status":"deployed"`} {
		if !strings.Contains(got, want) {
			t.Errorf("status output lacks %s: %s", want, got)
		}
	}
}

// TestStatusRejectsNonJSON: helm output that is not a release document is an
// error and is not echoed.
func TestStatusRejectsNonJSON(t *testing.T) {
	out := fakeHelm(t)
	writeFile(t, filepath.Join(out, "status.json"), "password: needle-value-one\n", 0o600)
	got, err := Status(context.Background(), "rel", "")
	if err == nil || strings.Contains(err.Error()+got, needleValue) {
		t.Errorf("got %q, %v; want an error that does not echo helm's output", got, err)
	}
}

// TestHelmRunsWithCleanEnv (S2): neither NSELF_* secrets nor a caller's
// HELM_DEBUG reach helm, for install, upgrade and status.
func TestHelmRunsWithCleanEnv(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", needleLicence)
	t.Setenv("NSELF_OTHER_SECRET", "needle-other")
	t.Setenv("HELM_DEBUG", "1")
	t.Setenv("KUBECONFIG", "/kept/kubeconfig")
	check := func(t *testing.T, out string) {
		t.Helper()
		env, err := os.ReadFile(filepath.Join(out, "env"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(env)
		if strings.Contains(s, "NSELF_") || strings.Contains(s, needleLicence) || strings.Contains(s, "HELM_DEBUG=1") ||
			strings.Contains(s, "HELM_DEBUG=true") {
			t.Errorf("helm env leaks NSELF_* or enables debug:\n%s", s)
		}
		if !strings.Contains(s, "HELM_DEBUG=false") || !strings.Contains(s, "KUBECONFIG=/kept/kubeconfig") {
			t.Errorf("helm env lost HELM_DEBUG=false or KUBECONFIG:\n%s", s)
		}
	}
	for name, fn := range map[string]func(context.Context, InstallOptions) error{"install": Install, "upgrade": Upgrade} {
		t.Run(name, func(t *testing.T) {
			out := fakeHelm(t)
			if err := fn(context.Background(), baseOpts(project(t, true, true))); err != nil {
				t.Fatal(err)
			}
			check(t, out)
		})
	}
	t.Run("status", func(t *testing.T) {
		out := fakeHelm(t)
		writeFile(t, filepath.Join(out, "status.json"), releaseJSON, 0o600)
		if _, err := Status(context.Background(), "rel", ""); err != nil {
			t.Fatal(err)
		}
		check(t, out)
	})
}

// TestHelmEnvKeepsOthers: the filter drops only NSELF_* and HELM_DEBUG.
func TestHelmEnvKeepsOthers(t *testing.T) {
	got := strings.Join(helmEnv([]string{"PATH=/bin", "NSELF_X=1", "HELM_DEBUG=1", "HELM_KUBECONTEXT=c", "HOME=/h"}), " ")
	if got != "PATH=/bin HELM_KUBECONTEXT=c HOME=/h HELM_DEBUG=false" {
		t.Errorf("helmEnv = %q", got)
	}
}

// TestCancelLeavesNothingBehind (S1): when the run is cancelled mid-helm (what
// a SIGINT/SIGTERM handler does), no chart or licence-bearing file is left in
// the temp directory.
func TestCancelLeavesNothingBehind(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stand-in for helm")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "helm"), "#!/bin/sh\ntouch \"$FAKE_STARTED\"\nexec sleep 30\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	started := filepath.Join(t.TempDir(), "started")
	t.Setenv("FAKE_STARTED", started)
	ctx, cancel := context.WithCancel(context.Background())
	opts := baseOpts(project(t, true, true))
	opts.LicenseKey = needleLicence
	done := make(chan error, 1)
	go func() { done <- Install(ctx, opts) }()
	deadline := time.Now().Add(10 * time.Second)
	for _, err := os.Stat(started); err != nil; _, err = os.Stat(started) {
		if time.Now().After(deadline) {
			t.Fatal("stand-in helm never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// While helm runs, nothing under the temp dir may hold the licence key.
	err := filepath.Walk(tmp, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte(needleLicence)) {
				t.Errorf("licence key on disk during the run: %s", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Install did not return after cancel")
	}
	left, _ := os.ReadDir(tmp)
	if len(left) != 0 {
		t.Errorf("temp dir not empty after cancel: %v", left)
	}
}

// TestInstallFailureHint (S4): a failed install says how to leave the failed
// release; an upgrade failure does not suggest uninstalling.
func TestInstallFailureHint(t *testing.T) {
	fakeHelm(t)
	t.Setenv("FAKE_HELM_FAIL", "1")
	err := Install(context.Background(), baseOpts(project(t, true, true)))
	if err == nil || !strings.Contains(err.Error(), "helm uninstall rel") || !strings.Contains(err.Error(), "nself k8s install") {
		t.Errorf("install error lacks the recovery hint: %v", err)
	}
	err = Upgrade(context.Background(), baseOpts(project(t, true, true)))
	if err == nil || strings.Contains(err.Error(), "uninstall") {
		t.Errorf("upgrade error = %v, want no uninstall hint", err)
	}
}

// TestUnreadableValuesIsNotMissing (N2): a permission error is reported as
// such, not as "run nself k8s values".
func TestUnreadableValuesIsNotMissing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs permission bits and a non-root user")
	}
	dir := project(t, true, true)
	gen := filepath.Join(dir, filepath.FromSlash(GeneratedDir))
	if err := os.Chmod(gen, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gen, 0o755) })
	_, err := ResolveValues(dir)
	if err == nil || errors.Is(err, ErrValuesMissing) || strings.Contains(err.Error(), "run nself k8s values") {
		t.Errorf("err = %v, want the permission error, not 'run nself k8s values'", err)
	}
}

// TestAnnounceTarget (N1): the kubeconfig helm will use is named.
func TestAnnounceTarget(t *testing.T) {
	t.Setenv("KUBECONFIG", "/env/kc")
	var b bytes.Buffer
	announceTarget(&b, "/flag/kc")
	announceTarget(&b, "")
	if s := b.String(); !strings.Contains(s, "/flag/kc") || !strings.Contains(s, "/env/kc") {
		t.Errorf("announce = %q", s)
	}
}
