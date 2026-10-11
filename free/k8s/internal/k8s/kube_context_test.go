// Purpose: tests for the explicit kube context (D-0376): the --kube-context
// flag, stripping every inherited HELM_KUBE* variable, the target
// announcement and URL redaction.
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

// TestHelmEnvFor: every HELM_KUBE* variable is stripped only with an explicit
// context; the rest of the filtering is unchanged.
func TestHelmEnvFor(t *testing.T) {
	parent := []string{"PATH=/bin", "NSELF_X=1", "HELM_DEBUG=1", "HELM_KUBECONTEXT=other",
		"HELM_KUBEAPISERVER=https://fake.example.invalid:6443", "HELM_KUBETOKEN=fake-token", "HELM_KUBEASUSER=fake-user", "HELM_KUBECAFILE=/fake/ca", "HELM_NAMESPACE=ns", "HOME=/h"}
	cases := []struct {
		name string
		ctx  string
		want string
	}{
		{"no context keeps inherited", "", "PATH=/bin HELM_KUBECONTEXT=other HELM_KUBEAPISERVER=https://fake.example.invalid:6443 HELM_KUBETOKEN=fake-token HELM_KUBEASUSER=fake-user HELM_KUBECAFILE=/fake/ca HELM_NAMESPACE=ns HOME=/h HELM_DEBUG=false"},
		{"explicit context strips", "fake-ctx", "PATH=/bin HELM_NAMESPACE=ns HOME=/h HELM_DEBUG=false"},
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
// --kube-context and no inherited HELM_KUBECONTEXT, HELM_KUBEAPISERVER or
// HELM_KUBETOKEN.
func TestUpgradeKubeContextOverridesEnv(t *testing.T) {
	for _, verb := range []string{"install", "upgrade"} {
		t.Run(verb, func(t *testing.T) {
			out := fakeHelm(t)
			t.Setenv("HELM_KUBECONTEXT", "other")
			t.Setenv("HELM_KUBEAPISERVER", "https://fake.example.invalid:6443")
			t.Setenv("HELM_KUBETOKEN", "fake-token")
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
			if s := string(env); strings.Contains(s, "HELM_KUBECONTEXT") || strings.Contains(s, "HELM_KUBEAPISERVER") || strings.Contains(s, "HELM_KUBETOKEN") {
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

// TestRedactURL: userinfo, query and fragment never survive; a non-URL is not
// echoed.
func TestRedactURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://user:fake-pass@fake.example.invalid:6443", "https://fake.example.invalid:6443"},
		{"https://fake.example.invalid:6443/?token=fake-tok#frag", "https://fake.example.invalid:6443/"},
		{"https://fake.example.invalid:6443?", "https://fake.example.invalid:6443"},
		{"user:fake-pass@host", "(unparsable value)"},
	}
	for _, c := range cases {
		if got := redactURL(c.in); got != c.want {
			t.Errorf("redactURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
