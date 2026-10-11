// Purpose: tests for the licence Secret flow (D-0311): the overlay carries the
// Secret NAME only, the key never reaches helm, and a bad name stops early.
//
// Inputs: the fake helm and project helpers of k8s_test.go.
//
// Outputs: test results.
//
// Constraints: no real helm, no cluster.
package k8s

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOverlayCarriesLicenseSecretNameOnly: --license-secret reaches helm as
// license.secretName on stdin; the key value reaches neither stdin nor argv; a
// bad name stops before helm runs.
func TestOverlayCarriesLicenseSecretNameOnly(t *testing.T) {
	out := fakeHelm(t)
	opts := baseOpts(project(t, true, true))
	opts.LicenseKey = needleLicence
	opts.LicenseSecret = "nself-license"
	if err := Upgrade(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "stdin"))
	if err != nil {
		t.Fatalf("no overlay on stdin: %v", err)
	}
	var got struct {
		License struct {
			SecretName string `yaml:"secretName"`
		} `yaml:"license"`
	}
	if err := yaml.Unmarshal(b, &got); err != nil || got.License.SecretName != "nself-license" {
		t.Errorf("overlay = %q (%v), want license.secretName", b, err)
	}
	if strings.Contains(string(b), needleLicence) {
		t.Errorf("licence key reached helm: %q", b)
	}
	for _, a := range argvOf(t, out) {
		if strings.Contains(a, needleLicence) || strings.Contains(a, "nself-license") {
			t.Errorf("licence key or Secret name on argv: %q", a)
		}
	}
	bad := fakeHelm(t)
	opts.LicenseSecret = "Not_Valid"
	if err := Install(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "--license-secret") {
		t.Errorf("err = %v, want a --license-secret refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(bad, "argv")); statErr == nil {
		t.Error("helm ran with an invalid licence Secret name")
	}
}
