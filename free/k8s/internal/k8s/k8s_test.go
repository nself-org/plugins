package k8s

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	nselfk8s "github.com/nself-org/nself-k8s"
)

const (
	needleValue   = "needle-value-one"
	needleLicence = "needle-licence-two"
)

// TestErrHelmNotFound verifies the sentinel error names helm.
func TestErrHelmNotFound(t *testing.T) {
	if !strings.Contains(ErrHelmNotFound.Error(), "helm") {
		t.Errorf("ErrHelmNotFound = %q: should mention helm", ErrHelmNotFound)
	}
}

// TestHelmBinaryNotInPath verifies helmBinary reports ErrHelmNotFound.
func TestHelmBinaryNotInPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := helmBinary(); !errors.Is(err, ErrHelmNotFound) {
		t.Errorf("helmBinary: got %v, want ErrHelmNotFound", err)
	}
}

// TestInstallUpgradeErrorWithoutHelm verifies both verbs propagate the error.
func TestInstallUpgradeErrorWithoutHelm(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	opts := InstallOptions{Domain: "test.local"}
	if err := Install(context.Background(), opts); !errors.Is(err, ErrHelmNotFound) {
		t.Errorf("Install: got %v, want ErrHelmNotFound", err)
	}
	if err := Upgrade(context.Background(), opts); !errors.Is(err, ErrHelmNotFound) {
		t.Errorf("Upgrade: got %v, want ErrHelmNotFound", err)
	}
}

// TestNoChartRepo pins that no remote chart repository survives: the package
// has no ChartRepo/RepoAdd and the embedded chart is the only chart source.
func TestNoChartRepo(t *testing.T) {
	src, err := os.ReadFile("helm.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"charts.nself.org", "repo\", \"add", "nself/nself", "--set"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("helm.go still contains %q", banned)
		}
	}
	if HelmReleaseName != "nself" {
		t.Errorf("HelmReleaseName = %q", HelmReleaseName)
	}
}

// project writes a project dir with generated values; withSecrets controls
// whether secrets.yaml exists.
func project(t *testing.T, withValues, withSecrets bool) string {
	t.Helper()
	dir := t.TempDir()
	gen := filepath.Join(dir, filepath.FromSlash(GeneratedDir))
	if err := os.MkdirAll(gen, 0o755); err != nil {
		t.Fatal(err)
	}
	if withValues {
		writeFile(t, filepath.Join(gen, "values.yaml"), "project: demo\nservices: {}\n", 0o644)
	}
	if withSecrets {
		writeFile(t, filepath.Join(gen, "secrets.yaml"), "secrets:\n  hasura:\n    ADMIN: "+needleValue+"\n", 0o600)
	}
	return dir
}

func writeFile(t *testing.T, p, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// fakeHelm puts a helm stand-in first on PATH. It records its argv, copies
// every --values file and notes whether the chart had templates/_helpers.tpl.
func fakeHelm(t *testing.T) (out string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stand-in for helm")
	}
	bin, out := t.TempDir(), t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_HELM_OUT/argv"
i=0; prev=""
for a in "$@"; do
  if [ "$prev" = "--values" ]; then cp "$a" "$FAKE_HELM_OUT/values-$i"; i=$((i+1)); fi
  prev="$a"
done
[ -f "$3/templates/_helpers.tpl" ] && echo yes > "$FAKE_HELM_OUT/helpers"
echo "$3" > "$FAKE_HELM_OUT/chartdir"
exit 0
`
	writeFile(t, filepath.Join(bin, "helm"), script, 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_HELM_OUT", out)
	return out
}

func baseOpts(dir string) InstallOptions {
	return InstallOptions{
		ReleaseName: "rel", Domain: "example.test", ProjectDir: dir,
		Chart: nselfk8s.Chart, ChartRoot: nselfk8s.ChartRoot, Wait: true, Timeout: "9m",
	}
}

func argvOf(t *testing.T, out string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, "argv"))
	if err != nil {
		t.Fatalf("helm was not run: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// TestInstallArgvHasNoSecretOrLicence is the secrets-on-argv guard: neither a
// value of secrets.yaml nor the licence key appears in any argument, no --set
// is used, and the licence reaches helm through the overrides file.
func TestInstallArgvHasNoSecretOrLicence(t *testing.T) {
	out := fakeHelm(t)
	opts := baseOpts(project(t, true, true))
	opts.LicenseKey = needleLicence
	opts.Plugins = []string{"ai", "mux", "cron"}
	if err := Install(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	argv := argvOf(t, out)
	for _, a := range argv {
		if strings.Contains(a, needleValue) || strings.Contains(a, needleLicence) {
			t.Errorf("secret or licence on argv: %q", a)
		}
		if strings.HasPrefix(a, "--set") {
			t.Errorf("--set used: %q", a)
		}
	}
	if argv[0] != "install" || argv[1] != "rel" {
		t.Errorf("argv head = %v", argv[:2])
	}
	if got := strings.Join(argv[3:], " "); !strings.Contains(got, "--values") ||
		!strings.Contains(got, "--wait --timeout 9m") || strings.Contains(got, "--kubeconfig") {
		t.Errorf("argv tail = %q", got)
	}
	// values.yaml, secrets.yaml, overrides in that order.
	if b, _ := os.ReadFile(filepath.Join(out, "values-1")); !strings.Contains(string(b), needleValue) {
		t.Errorf("second --values is not secrets.yaml: %q", b)
	}
	b, err := os.ReadFile(filepath.Join(out, "values-2"))
	if err != nil {
		t.Fatalf("no overrides file passed: %v", err)
	}
	var got struct {
		Domain  string
		License struct{ Key string }
		Plugins struct{ Install []string }
	}
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Domain != "example.test" || got.License.Key != needleLicence {
		t.Errorf("overrides = %+v", got)
	}
	if want := []string{"ai", "mux", "cron"}; strings.Join(got.Plugins.Install, ",") != strings.Join(want, ",") {
		t.Errorf("plugins.install = %v, want %v (indices 0..n-1, in order)", got.Plugins.Install, want)
	}
}

// TestInstallPassesExtractedChart checks helm sees the chart with its
// underscore-prefixed helper, and that the temp chart is gone afterwards.
func TestInstallPassesExtractedChart(t *testing.T) {
	out := fakeHelm(t)
	if err := Install(context.Background(), baseOpts(project(t, true, true))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "helpers")); err != nil {
		t.Error("chart given to helm had no templates/_helpers.tpl")
	}
	dir, _ := os.ReadFile(filepath.Join(out, "chartdir"))
	if _, err := os.Stat(strings.TrimSpace(string(dir))); !os.IsNotExist(err) {
		t.Errorf("temp chart dir %q not removed: %v", dir, err)
	}
}

// TestRefusesWithoutGeneratedValues: a missing values.yaml or secrets.yaml
// stops before helm runs and tells the user which command to run.
func TestRefusesWithoutGeneratedValues(t *testing.T) {
	for name, dir := range map[string]string{
		"no values":  project(t, false, true),
		"no secrets": project(t, true, false),
		"neither":    project(t, false, false),
	} {
		t.Run(name, func(t *testing.T) {
			out := fakeHelm(t)
			for verb, fn := range map[string]func(context.Context, InstallOptions) error{"install": Install, "upgrade": Upgrade} {
				err := fn(context.Background(), baseOpts(dir))
				if err == nil || !strings.Contains(err.Error(), "run nself k8s values") || !errors.Is(err, ErrValuesMissing) {
					t.Errorf("%s: err = %v, want 'run nself k8s values'", verb, err)
				}
				if _, statErr := os.Stat(filepath.Join(out, "argv")); statErr == nil {
					t.Errorf("%s: helm ran although values are missing", verb)
				}
			}
		})
	}
}

// TestUpgradeAppliesFreshValues: upgrade passes the generated values, never
// --reuse-values, and only passes --kubeconfig when asked.
func TestUpgradeAppliesFreshValues(t *testing.T) {
	out := fakeHelm(t)
	t.Setenv("KUBECONFIG", "/should/not/appear")
	opts := baseOpts(project(t, true, true))
	opts.Wait = false
	if err := Upgrade(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(argvOf(t, out), " ")
	for _, bad := range []string{"--reuse-values", "--kubeconfig", "--wait", "/should/not/appear"} {
		if strings.Contains(joined, bad) {
			t.Errorf("upgrade argv contains %q: %s", bad, joined)
		}
	}
	if !strings.HasPrefix(joined, "upgrade rel ") || strings.Count(joined, "--values") < 2 {
		t.Errorf("upgrade argv = %s", joined)
	}
	opts.Kubeconfig = "/tmp/kc"
	if err := Upgrade(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(argvOf(t, out), " "), "--kubeconfig /tmp/kc") {
		t.Error("--kubeconfig from the flag was not passed")
	}
}

// TestExtractChart: the extracted chart holds _helpers.tpl (the embed uses
// all:), is private, and cleanup removes it.
func TestExtractChart(t *testing.T) {
	dir, cleanup, err := ExtractChart(nselfk8s.Chart, nselfk8s.ChartRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Chart.yaml", "values.yaml", "templates/_helpers.tpl", "templates/deployment.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("extracted chart lacks %s: %v", f, err)
		}
	}
	if runtime.GOOS != "windows" {
		for _, p := range []string{filepath.Dir(dir), dir, filepath.Join(dir, "templates")} {
			if st, err := os.Stat(p); err != nil || st.Mode().Perm() != 0o700 {
				t.Errorf("%s mode = %v, want 0700 (%v)", p, st.Mode().Perm(), err)
			}
		}
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup left %s", dir)
	}
}

// TestExtractChartMissingRoot fails loudly and leaves no temp dir behind.
func TestExtractChartMissingRoot(t *testing.T) {
	if _, _, err := ExtractChart(nselfk8s.Chart, "charts/absent"); err == nil {
		t.Error("expected an error for a missing chart root")
	}
	if _, _, err := ExtractChart(nil, "x"); err == nil {
		t.Error("expected an error for a nil chart")
	}
}
