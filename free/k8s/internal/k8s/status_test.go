// Purpose: tests for the release-not-found classification of helm's stderr.
//
// Inputs: sample helm stderr lines.
//
// Outputs: test results.
//
// Constraints: no helm, no cluster.
package k8s

import "testing"

func TestReleaseMissing(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"status", "Error: release: not found\n", true},
		{"uninstall", "Error: uninstall: Release not loaded: nself: release: not found\n", true},
		{"mixed case", "ERROR: Release: Not Found", true},
		{"unknown kube context", "Error: Kubernetes cluster unreachable: context \"prod\" not found\n", false},
		{"namespace", "Error: namespaces \"x\" not found", false},
		{"kubeconfig file", "Error: stat /k/cfg: no such file or directory", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		if got := releaseMissing([]byte(c.stderr)); got != c.want {
			t.Errorf("%s: releaseMissing(%q) = %v, want %v", c.name, c.stderr, got, c.want)
		}
	}
}
