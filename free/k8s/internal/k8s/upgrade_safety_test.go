// Purpose: tests for safe upgrades (D-0315) and an explicit kube context
// (D-0376): --atomic argv, the rollback hint, helm's stderr in status errors,
// the kube context flag, env stripping and the target announcement.
//
// Inputs: the fakeHelm stand-in (k8s_test.go) and fake values only.
//
// Outputs: test results.
//
// Constraints: no real helm, no cluster; every URL and token is an invalid
// placeholder.
package k8s

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
			if got := countArg(argv, "--atomic") == 1; got != c.wantAtomic || countArg(argv, "--atomic") > 1 {
				t.Errorf("--atomic count = %d, want present=%v: %v", countArg(argv, "--atomic"), c.wantAtomic, argv)
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

// TestBuildArgsKubeContext: --kube-context follows --kubeconfig, only when set.
func TestBuildArgsKubeContext(t *testing.T) {
	vf := ValuesFiles{Values: "/v.yaml", Secrets: "/s.yaml"}
	argv := buildArgs("install", InstallOptions{Kubeconfig: "/kc", KubeContext: "fake-ctx"}, "/chart", vf, false)
	if tail := strings.Join(argv[len(argv)-4:], " "); tail != "--kubeconfig /kc --kube-context fake-ctx" {
		t.Errorf("argv tail = %q", tail)
	}
	if argv := buildArgs("install", InstallOptions{}, "/chart", vf, false); countArg(argv, "--kube-context") != 0 {
		t.Errorf("--kube-context without a context: %v", argv)
	}
}

// TestUpgradeFailureNamesRollback: a failed upgrade's Hint says helm rolled
// back and how to check or redo the rollback, offers no uninstall, and
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
			_, err := ReleaseStatus(context.Background(), "rel", "")
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
		_, err := ReleaseStatus(context.Background(), "rel", "")
		if he, ok := err.(*HelmError); !ok || !he.NotFound || he.Stderr != "Error: release: not found" {
			t.Errorf("err = %#v", err)
		}
	})
}

// TestHelmEnvFor: HELM_KUBECONTEXT and HELM_KUBEAPISERVER are stripped only
// with an explicit context; the rest of the filtering is unchanged.
func TestHelmEnvFor(t *testing.T) {
	parent := []string{"PATH=/bin", "NSELF_X=1", "HELM_DEBUG=1", "HELM_KUBECONTEXT=other",
		"HELM_KUBEAPISERVER=https://fake.example.invalid:6443", "HELM_KUBETOKEN=fake-token", "HOME=/h"}
	cases := []struct {
		name string
		ctx  string
		want string
	}{
		{"no context keeps inherited", "", "PATH=/bin HELM_KUBECONTEXT=other HELM_KUBEAPISERVER=https://fake.example.invalid:6443 HELM_KUBETOKEN=fake-token HOME=/h HELM_DEBUG=false"},
		{"explicit context strips", "fake-ctx", "PATH=/bin HELM_KUBETOKEN=fake-token HOME=/h HELM_DEBUG=false"},
	}
	for _, c := range cases {
		if got := strings.Join(helmEnvFor(parent, c.ctx), " "); got != c.want {
			t.Errorf("%s: helmEnvFor = %q, want %q", c.name, got, c.want)
		}
	}
	if got, want := strings.Join(helmEnv(parent), " "), strings.Join(helmEnvFor(parent, ""), " "); got != want {
		t.Errorf("helmEnv = %q, want %q", got, want)
	}
}

// TestUpgradeKubeContextOverridesEnv: with an explicit context helm gets
// --kube-context and no inherited HELM_KUBECONTEXT or HELM_KUBEAPISERVER.
func TestUpgradeKubeContextOverridesEnv(t *testing.T) {
	for _, verb := range []string{"install", "upgrade"} {
		t.Run(verb, func(t *testing.T) {
			out := fakeHelm(t)
			t.Setenv("HELM_KUBECONTEXT", "other")
			t.Setenv("HELM_KUBEAPISERVER", "https://fake.example.invalid:6443")
			opts := baseOpts(project(t, true, true))
			opts.KubeContext = "fake-ctx"
			fn := Upgrade
			if verb == "install" {
				fn = Install
			}
			if err := fn(context.Background(), opts); err != nil {
				t.Fatal(err)
			}
			if joined := strings.Join(argvOf(t, out), " "); !strings.Contains(joined, "--kube-context fake-ctx") {
				t.Errorf("argv lacks --kube-context fake-ctx: %s", joined)
			}
			env, err := os.ReadFile(filepath.Join(out, "env"))
			if err != nil {
				t.Fatal(err)
			}
			if s := string(env); strings.Contains(s, "HELM_KUBECONTEXT") || strings.Contains(s, "HELM_KUBEAPISERVER") {
				t.Errorf("helm env kept an inherited cluster selector:\n%s", s)
			}
		})
	}
}

// TestAnnounceTargetContext: the context and API server line for each source,
// with userinfo removed and HELM_KUBETOKEN never printed.
func TestAnnounceTargetContext(t *testing.T) {
	const api = "https://user:fake-pass@fake.example.invalid:6443"
	cases := []struct {
		name    string
		flagCtx string
		envCtx  string
		envAPI  string
		want    []string
		absent  []string
	}{
		{"flag ctx, api set", "fake-ctx", "env-ctx", api,
			[]string{"k8s: target context fake-ctx (--context)\n"}, []string{"API server", "inherited"}},
		{"flag ctx, no api", "fake-ctx", "", "",
			[]string{"k8s: target context fake-ctx (--context)\n"}, []string{"API server"}},
		{"env ctx, api set", "", "env-ctx", api,
			[]string{"k8s: target context env-ctx (inherited HELM_KUBECONTEXT)\n",
				"k8s: target API server https://fake.example.invalid:6443 (inherited HELM_KUBEAPISERVER)\n"}, nil},
		{"env ctx, no api", "", "env-ctx", "",
			[]string{"k8s: target context env-ctx (inherited HELM_KUBECONTEXT)\n"}, []string{"API server"}},
		{"none, api set", "", "", api,
			[]string{"k8s: target API server https://fake.example.invalid:6443 (inherited HELM_KUBEAPISERVER)\n"}, nil},
		{"none, no api", "", "", "", nil, []string{"API server", "context"}},
		{"unparsable api", "", "", "user:fake-pass@host",
			[]string{"k8s: target API server (unparsable value) (inherited HELM_KUBEAPISERVER)\n"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("KUBECONFIG", "/env/kc")
			t.Setenv("HELM_KUBECONTEXT", c.envCtx)
			t.Setenv("HELM_KUBEAPISERVER", c.envAPI)
			t.Setenv("HELM_KUBETOKEN", "fake-token")
			var b bytes.Buffer
			announceTarget(&b, "", c.flagCtx)
			s := b.String()
			if !strings.HasPrefix(s, "k8s: target kubeconfig /env/kc (KUBECONFIG)\n") {
				t.Errorf("kubeconfig line missing: %q", s)
			}
			if c.name == "none, no api" && s != "k8s: target kubeconfig /env/kc (KUBECONFIG)\n" {
				t.Errorf("output = %q, want exactly the kubeconfig line", s)
			}
			for _, w := range c.want {
				if !strings.Contains(s, w) {
					t.Errorf("output lacks %q:\n%s", w, s)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(s, a) {
					t.Errorf("output contains %q:\n%s", a, s)
				}
			}
			for _, secret := range []string{"fake-pass", "fake-token", "user:"} {
				if strings.Contains(s, secret) {
					t.Errorf("output leaks %q:\n%s", secret, s)
				}
			}
		})
	}
}
