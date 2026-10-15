// Purpose: tests for safe upgrades (D-0315): --atomic argv, the rollback hint,
// and helm's stderr in status errors.
//
// Inputs: the fakeHelm stand-in (k8s_test.go) and fake values only.
//
// Outputs: test results.
//
// Constraints: no real helm, no cluster; every URL and token is an invalid
// placeholder.
package k8s

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// countArg returns how many times want appears in argv.
func countArg(argv []string, want string) int {
	n := 0
	for _, a := range argv {
		if a == want {
			n++
		}
	}
	return n
}

// TestBuildArgsAtomicWait: upgrade is always atomic and waits; install waits
// only when asked; --wait appears at most once.
func TestBuildArgsAtomicWait(t *testing.T) {
	cases := []struct {
		name        string
		verb        string
		wait        bool
		timeout     string
		wantAtomic  bool
		wantWait    int
		wantTimeout string
	}{
		{"install no wait", "install", false, "", false, 0, ""},
		{"install wait", "install", true, "", false, 1, DefaultTimeout},
		{"install wait custom", "install", true, "3m", false, 1, "3m"},
		{"upgrade no wait", "upgrade", false, "", true, 1, DefaultTimeout},
		{"upgrade wait", "upgrade", true, "", true, 1, DefaultTimeout},
		{"upgrade custom timeout", "upgrade", false, "7m", true, 1, "7m"},
		{"upgrade wait custom timeout", "upgrade", true, "7m", true, 1, "7m"},
	}
	vf := ValuesFiles{Values: "/v.yaml", Secrets: "/s.yaml"}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := InstallOptions{ReleaseName: "rel", Wait: c.wait, Timeout: c.timeout}
			argv := buildArgs(c.verb, opts, "/chart", vf, false)
			wantAtomic := 0
			if c.wantAtomic {
				wantAtomic = 1
			}
			if got := countArg(argv, "--atomic"); got != wantAtomic {
				t.Errorf("--atomic count = %d, want %d: %v", got, wantAtomic, argv)
			}
			if got := countArg(argv, "--wait"); got != c.wantWait {
				t.Errorf("--wait count = %d, want %d: %v", got, c.wantWait, argv)
			}
			if c.wantTimeout == "" {
				if countArg(argv, "--timeout") != 0 {
					t.Errorf("unexpected --timeout: %v", argv)
				}
				return
			}
			joined := " " + strings.Join(argv, " ") + " "
			if !strings.Contains(joined, " --timeout "+c.wantTimeout+" ") {
				t.Errorf("want --timeout %s: %v", c.wantTimeout, argv)
			}
		})
	}
}

// TestUpgradeFailureNamesRollback: a failed upgrade's Hint says how to check
// or redo helm's rollback, offers no uninstall, and
// Error() keeps the text cmd prints today (D-0315).
func TestUpgradeFailureNamesRollback(t *testing.T) {
	fakeHelm(t)
	t.Setenv("FAKE_HELM_FAIL", "1")
	cases := []struct{ name, release, wantRel string }{
		{"named release", "rel", "rel"},
		{"default release", "", HelmReleaseName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := baseOpts(project(t, true, true))
			opts.ReleaseName = c.release
			err := Upgrade(context.Background(), opts)
			he, ok := err.(*HelmError)
			if !ok || he.Verb != "upgrade" {
				t.Fatalf("error = %#v, want *HelmError with Verb upgrade", err)
			}
			if he.Error() != "k8s: helm upgrade: exit status 1" {
				t.Errorf("Error() = %q, want the unchanged text", he.Error())
			}
			for _, want := range []string{"--atomic", "helm history " + c.wantRel, "helm rollback " + c.wantRel} {
				if !strings.Contains(he.Hint, want) {
					t.Errorf("Hint lacks %q: %q", want, he.Hint)
				}
			}
			if strings.Contains(he.Hint+he.Error(), "uninstall") {
				t.Errorf("upgrade failure suggests uninstall: %q", he.Hint)
			}
		})
	}
}

// TestReleaseStatusErrorCarriesStderr: a status failure keeps helm's trimmed
// stderr, capped to 512 bytes, in HelmError.Stderr; Error() is unchanged.
func TestReleaseStatusErrorCarriesStderr(t *testing.T) {
	long := strings.Repeat("x", 600) + "END"
	cases := []struct {
		name   string
		stderr string
		check  func(t *testing.T, got string)
	}{
		{"line", "  Error: Kubernetes cluster unreachable: fake\n\n", func(t *testing.T, got string) {
			if got != "Error: Kubernetes cluster unreachable: fake" {
				t.Errorf("Stderr = %q, want the trimmed line", got)
			}
		}},
		{"capped", long, func(t *testing.T, got string) {
			if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "END") || len(got) != 3+512 {
				t.Errorf("Stderr len %d, want 515 starting with ... and ending END: %q", len(got), got)
			}
		}},
		{"rune boundary", strings.Repeat("é", 400), func(t *testing.T, got string) {
			tail := strings.TrimPrefix(got, "...")
			if !strings.HasPrefix(got, "...") || !strings.HasPrefix(tail, "é") || len(tail) > 512 {
				t.Errorf("Stderr %q (len %d) not cut at a rune boundary", got, len(tail))
			}
		}},
		{"empty", "", func(t *testing.T, got string) {
			if got != "" {
				t.Errorf("Stderr = %q, want empty", got)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeHelm(t)
			t.Setenv("FAKE_HELM_FAIL", "1")
			t.Setenv("FAKE_HELM_STDERR", c.stderr)
			_, err := ReleaseStatus(context.Background(), "rel", "", "")
			he, ok := err.(*HelmError)
			if !ok {
				t.Fatalf("err = %#v, want *HelmError", err)
			}
			if he.Error() != "k8s: helm status: exit status 1" {
				t.Errorf("Error() = %q, want the unchanged text", he.Error())
			}
			c.check(t, he.Stderr)
		})
	}
	t.Run("not found still detected", func(t *testing.T) {
		fakeHelm(t)
		t.Setenv("FAKE_HELM_FAIL", "1")
		t.Setenv("FAKE_HELM_STDERR", "Error: release: not found\n")
		_, err := ReleaseStatus(context.Background(), "rel", "", "")
		if he, ok := err.(*HelmError); !ok || !he.NotFound || he.Stderr != "Error: release: not found" {
			t.Errorf("err = %#v", err)
		}
	})
}

// TestUpgradeHintCarriesCluster: the suggested helm commands name the caller's
// kubeconfig and context; with neither the reminder stays.
func TestUpgradeHintCarriesCluster(t *testing.T) {
	cases := []struct {
		name, kubeconfig, ctx string
		wantFlags             string
		wantReminder          bool
	}{
		{"neither", "", "", "", true},
		{"kubeconfig", "/fake/kc", "", " --kubeconfig /fake/kc", false},
		{"context", "", "fake-ctx", " --kube-context fake-ctx", false},
		{"both", "/fake/kc", "fake-ctx", " --kubeconfig /fake/kc --kube-context fake-ctx", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := InstallOptions{ReleaseName: "rel", Kubeconfig: c.kubeconfig, KubeContext: c.ctx}
			he, ok := runError("upgrade", opts, errors.New("boom")).(*HelmError)
			if !ok {
				t.Fatal("want *HelmError")
			}
			for _, want := range []string{"'helm history rel" + c.wantFlags + "'", "'helm rollback rel" + c.wantFlags + "'"} {
				if !strings.Contains(he.Hint, want) {
					t.Errorf("Hint lacks %q: %q", want, he.Hint)
				}
			}
			if got := strings.Contains(he.Hint, "(add --kubeconfig for your cluster)"); got != c.wantReminder {
				t.Errorf("reminder present = %v, want %v: %q", got, c.wantReminder, he.Hint)
			}
			if !strings.HasPrefix(he.Hint, "If helm applied the upgrade, it rolled the release back") {
				t.Errorf("Hint must not claim a rollback unconditionally: %q", he.Hint)
			}
		})
	}
}
