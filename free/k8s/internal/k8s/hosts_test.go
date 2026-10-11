package k8s

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const valuesWithRules = `project: demo
services: {}
ingress:
  rules:
    - {name: b, host: b.example.test, service: hasura, port: 8080, scheme: http}
    - {name: a, host: a.example.test, service: auth, port: 4000, scheme: http}
    - {name: c, host: b.example.test, service: x, port: 1, scheme: http}
`

// TestIngressHosts: hosts come from ingress.rules, sorted and unique; no
// rules means no host; a missing values file keeps the "run nself k8s values" hint.
func TestIngressHosts(t *testing.T) {
	dir := project(t, true, true)
	hosts, err := IngressHosts(dir, "")
	if err != nil || len(hosts) != 0 {
		t.Fatalf("no rules: hosts=%v err=%v, want none", hosts, err)
	}
	writeFile(t, filepath.Join(dir, filepath.FromSlash(GeneratedDir), "values.yaml"), valuesWithRules, 0o644)
	hosts, err = IngressHosts(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(hosts, ","); got != "a.example.test,b.example.test" {
		t.Errorf("hosts = %q, want sorted unique a.example.test,b.example.test", got)
	}
	if _, err := IngressHosts(project(t, false, false), ""); err == nil || !strings.Contains(err.Error(), "run nself k8s values") {
		t.Errorf("missing values: err = %v", err)
	}
}

// TestNotPassed names exactly the empty install-time values.
func TestNotPassed(t *testing.T) {
	if got := NotPassed(InstallOptions{}); len(got) != 2 {
		t.Errorf("empty options: %v, want domain and plugins", got)
	}
	full := InstallOptions{Domain: "d", LicenseKey: "k", Plugins: []string{"ai"}}
	if got := NotPassed(full); len(got) != 0 {
		t.Errorf("full options: %v, want none", got)
	}
	if got := NotPassed(InstallOptions{Domain: "d", Plugins: []string{"ai"}}); len(got) != 0 {
		t.Errorf("the licence key is never passed, so it is not listed: %v", got)
	}
}

// TestOverlayNeverOnDisk: the install overlay reaches helm on stdin, no
// overlay file is written next to the chart, and the licence key and secrets
// stay off argv for install and upgrade alike.
func TestOverlayNeverOnDisk(t *testing.T) {
	for name, fn := range map[string]func(context.Context, InstallOptions) error{"install": Install, "upgrade": Upgrade} {
		t.Run(name, func(t *testing.T) {
			out := fakeHelm(t)
			opts := baseOpts(project(t, true, true))
			opts.LicenseKey = needleLicence
			if err := fn(context.Background(), opts); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(out, "stdin")); err != nil || !strings.Contains(string(b), "example.test") {
				t.Errorf("overlay not on stdin: %q, %v", b, err)
			}
			parent, _ := os.ReadFile(filepath.Join(out, "chartparent"))
			if got := strings.TrimSpace(string(parent)); got != "nself" {
				t.Errorf("chart temp dir holds %q, want only the chart", got)
			}
			for _, a := range argvOf(t, out) {
				if strings.Contains(a, needleValue) || strings.Contains(a, needleLicence) || strings.HasPrefix(a, "--set") {
					t.Errorf("%s: secret, licence or --set on argv: %q", name, a)
				}
			}
		})
	}
}

// TestRefusesGroupReadableSecrets: a secrets.yaml that group or others can
// read stops before helm runs.
func TestRefusesGroupReadableSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes")
	}
	out := fakeHelm(t)
	dir := project(t, true, true)
	if err := os.Chmod(filepath.Join(dir, filepath.FromSlash(GeneratedDir), "secrets.yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Install(context.Background(), baseOpts(dir))
	if err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Errorf("err = %v, want a chmod 600 refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(out, "argv")); statErr == nil {
		t.Error("helm ran with a group-readable secrets.yaml")
	}
}

// TestLicenseKeyUnsent: a licence key in the environment without a
// --license-secret is reported as not sent; a Secret name or no key is not.
func TestLicenseKeyUnsent(t *testing.T) {
	cases := map[string]struct {
		opts InstallOptions
		want bool
	}{
		"key only":       {InstallOptions{LicenseKey: "k"}, true},
		"key and secret": {InstallOptions{LicenseKey: "k", LicenseSecret: "nself-license"}, false},
		"secret only":    {InstallOptions{LicenseSecret: "nself-license"}, false},
		"neither":        {InstallOptions{}, false},
	}
	for name, c := range cases {
		if got := LicenseKeyUnsent(c.opts); got != c.want {
			t.Errorf("%s: LicenseKeyUnsent = %v, want %v", name, got, c.want)
		}
	}
	if strings.Contains(LicenseSecretHint, "=fake") || !strings.Contains(LicenseSecretHint, "--license-secret") {
		t.Errorf("hint = %q", LicenseSecretHint)
	}
}

// TestRewriteHost: the Go twin of the chart helper nself.host.
func TestRewriteHost(t *testing.T) {
	cases := []struct{ name, host, base, domain, want string }{
		{"apex", "example.test", "example.test", "shop.org", "shop.org"},
		{"subdomain keeps prefix", "api.example.test", "example.test", "shop.org", "api.shop.org"},
		{"nested prefix", "a.b.example.test", "example.test", "shop.org", "a.b.shop.org"},
		{"other host unchanged", "mail.other.test", "example.test", "shop.org", "mail.other.test"},
		{"suffix without dot is not a subdomain", "notexample.test", "example.test", "shop.org", "notexample.test"},
		{"no domain", "api.example.test", "example.test", "", "api.example.test"},
		{"no base domain", "api.example.test", "", "shop.org", "api.example.test"},
		{"same domain", "api.example.test", "example.test", "example.test", "api.example.test"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RewriteHost(c.host, c.base, c.domain); got != c.want {
				t.Errorf("RewriteHost(%q, %q, %q) = %q, want %q", c.host, c.base, c.domain, got, c.want)
			}
		})
	}
}

// TestIngressHostsAppliesDomain: with a baseDomain in the values and an install
// domain, the printed hosts are the rewritten ones, sorted and unique.
func TestIngressHostsAppliesDomain(t *testing.T) {
	dir := project(t, true, true)
	body := "baseDomain: example.test\n" + valuesWithRules + "    - {name: d, host: mail.other.test, service: m, port: 1, scheme: http}\n"
	writeFile(t, filepath.Join(dir, filepath.FromSlash(GeneratedDir), "values.yaml"), body, 0o644)
	hosts, err := IngressHosts(dir, "shop.org")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(hosts, ","); got != "a.shop.org,b.shop.org,mail.other.test" {
		t.Errorf("hosts = %q", got)
	}
	hosts, _ = IngressHosts(dir, "")
	if got := strings.Join(hosts, ","); got != "a.example.test,b.example.test,mail.other.test" {
		t.Errorf("no domain: hosts = %q", got)
	}
}
