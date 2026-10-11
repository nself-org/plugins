package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-k8s/internal/k8s"
)

func writeValues(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	gen := filepath.Join(dir, ".nself", "generated", "k8s")
	if err := os.MkdirAll(gen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gen, "values.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gen, "secrets.yaml"), []byte("secrets: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestInstalledLinePrintsIngressHosts: the success line lists the generated
// ingress hosts, rewritten to --domain when the values carry a baseDomain, and
// prints no URL when none is routed.
func TestInstalledLinePrintsIngressHosts(t *testing.T) {
	dir := writeValues(t, "ingress:\n  rules:\n    - {name: h, host: api.example.test, service: hasura, port: 8080, scheme: http}\n")
	got := installedLine(dir, "")
	if !strings.Contains(got, "https://api.example.test") {
		t.Errorf("line = %q, want the ingress host URL", got)
	}
	none := installedLine(writeValues(t, "ingress:\n  rules: []\n"), "")
	if strings.Contains(none, "https://") || !strings.Contains(none, "no ingress host") {
		t.Errorf("no rules: line = %q, want no URL", none)
	}
}

// TestInstalledLineAppliesDomain: with baseDomain in the values, --domain
// replaces it in the printed hosts, as the chart renders them.
func TestInstalledLineAppliesDomain(t *testing.T) {
	dir := writeValues(t, "baseDomain: example.test\ningress:\n  rules:\n    - {name: h, host: api.example.test, service: hasura, port: 8080, scheme: http}\n")
	if got := installedLine(dir, "shop.org"); !strings.Contains(got, "https://api.shop.org") || strings.Contains(got, "example.test") {
		t.Errorf("line = %q, want the rewritten host", got)
	}
}

// TestFlagHelpIsCurrent: the chart now reads domain, plugins.install and the
// licence Secret name, so no help text may say it does not (D-0311 is closed),
// and --license-secret is on both install and upgrade.
func TestFlagHelpIsCurrent(t *testing.T) {
	for _, c := range []*cobra.Command{installCmd, upgradeCmd} {
		for _, name := range []string{"domain", "plugins", "license-secret"} {
			f := c.Flags().Lookup(name)
			if f == nil {
				t.Errorf("%s has no --%s flag", c.Name(), name)
				continue
			}
			if strings.Contains(f.Usage, "D-0311") || strings.Contains(f.Usage, "does not consume") {
				t.Errorf("%s --%s help is stale: %q", c.Name(), name, f.Usage)
			}
		}
		if strings.Contains(c.Long, "D-0311") || strings.Contains(c.Long, "Not consumed") || strings.Contains(c.Long, "does not consume") {
			t.Errorf("%s Long text is stale", c.Name())
		}
	}
}

// TestLicenseSecretIsValidated: a bad Secret name is a usage error.
func TestLicenseSecretIsValidated(t *testing.T) {
	cases := map[string]bool{
		"nself-license": true, "a.b-c": true, "": true,
		"Upper": false, "-lead": false, "trail-": false, "has space": false, "a..b": false,
		strings.Repeat("a", 254): false,
	}
	for name, ok := range cases {
		if err := k8s.ValidateLicenseSecret(name); (err == nil) != ok {
			t.Errorf("ValidateLicenseSecret(%q) = %v, want ok=%v", name, err, ok)
		}
	}
}
